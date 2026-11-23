package group

import (
	errors2 "errors"
	"fmt"
	"strings"

	"github.com/noelbennettz/imail/consts"
	"github.com/noelbennettz/imail/db"
	"github.com/noelbennettz/imail/dto"
	"github.com/noelbennettz/imail/models"
	"github.com/noelbennettz/imail/services/del_email"
	"github.com/noelbennettz/imail/utils/array"
	"github.com/noelbennettz/imail/utils/context"
	"github.com/noelbennettz/imail/utils/errors"
	log "github.com/sirupsen/logrus"

	"xorm.io/builder"
)

type GroupItem struct {
	Id       int          `json:"id"`
	Label    string       `json:"label"`
	Tag      string       `json:"tag"`
	Unread   int          `json:"unread"`
	Children []*GroupItem `json:"children"`
}

func CreateGroup(ctx *context.Context, name string, parentId int) (*models.Group, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors2.New("group name is required")
	}
	if len([]rune(name)) > 255 {
		return nil, errors2.New("group name is too long")
	}
	if isReservedName(name) {
		return nil, errors2.New("mailbox name is reserved")
	}
	if parentId > 0 {
		var parent models.Group
		found, err := db.Instance.Where("id=? and user_id=?", parentId, ctx.UserID).Get(&parent)
		if err != nil {
			return nil, errors.Wrap(err)
		}
		if !found {
			return nil, errors2.New("parent group not found")
		}
	}
	// A leaf name only has to be unique inside its parent, so the same name
	// can be used under different parents (IMAP allows this).
	var group models.Group
	if _, err := db.Instance.Table("group").Where("name = ? and parent_id = ? and user_id = ?", name, parentId, ctx.UserID).Get(&group); err != nil {
		return nil, errors.Wrap(err)
	}
	if group.ID > 0 {
		return &group, nil
	}
	group.Name = name
	group.ParentId = parentId
	group.UserId = ctx.UserID
	group.FullPath = getLayerName(ctx, &group, true)

	_, err := db.Instance.Insert(&group)
	return &group, err
}

// RenameByPath renames (and optionally moves) a mailbox. Every descendant is
// rewritten in the same transaction so full_path never points at an old name.
func RenameByPath(ctx *context.Context, oldPath, newPath string) error {
	oldPath = strings.Trim(strings.TrimSpace(oldPath), "/")
	newPath = strings.Trim(strings.TrimSpace(newPath), "/")
	if oldPath == "" || newPath == "" {
		return errors2.New("mailbox name is required")
	}
	if len([]rune(oldPath)) > 600 || len([]rune(newPath)) > 600 || len([]rune(pathLeaf(newPath))) > 255 {
		return errors2.New("mailbox name is too long")
	}
	oldGroup, err := ResolveBox(ctx, oldPath)
	if err != nil {
		return err
	}
	if oldGroup == nil || oldGroup.ID == 0 {
		return errors2.New("group not found")
	}

	newName := strings.TrimSpace(pathLeaf(newPath))
	if isReservedName(newName) {
		return errors2.New("mailbox name is reserved")
	}
	newParentPath := pathParent(newPath)
	var newParent *models.Group
	if newParentPath != "" {
		newParent, err = ResolveBox(ctx, newParentPath)
		if err != nil {
			return err
		}
		if newParent == nil || newParent.ID == 0 {
			return errors2.New("parent group not found")
		}
	}

	oldFullPath := oldGroup.FullPath
	if oldFullPath == "" {
		oldFullPath = getLayerName(ctx, oldGroup, true)
	}
	parentID := 0
	if newParent != nil {
		parentID = newParent.ID
	}
	oldGroup.Name = newName
	oldGroup.ParentId = parentID
	oldGroup.FullPath = getLayerName(ctx, oldGroup, true)
	newFullPath := oldGroup.FullPath

	trans := db.Instance.NewSession()
	defer trans.Close()
	if err := trans.Begin(); err != nil {
		return errors.Wrap(err)
	}
	defer trans.Rollback()
	if _, err := trans.ID(oldGroup.ID).Cols("name", "parent_id", "full_path").Update(oldGroup); err != nil {
		return errors.Wrap(err)
	}
	var descendants []models.Group
	if err := trans.Where("user_id=? and full_path LIKE ?", ctx.UserID, oldFullPath+"/%").Find(&descendants); err != nil {
		return errors.Wrap(err)
	}
	for i := range descendants {
		oldDescPath := descendants[i].FullPath
		newDescPath := newFullPath + strings.TrimPrefix(oldDescPath, oldFullPath)
		descendants[i].FullPath = newDescPath
		if _, err := trans.ID(descendants[i].ID).Cols("full_path").Update(&descendants[i]); err != nil {
			return errors.Wrap(err)
		}
		if _, err := trans.Table(&models.MailboxState{}).Where("user_id=? and mailbox=?", ctx.UserID, oldDescPath).
			Update(map[string]interface{}{"mailbox": newDescPath}); err != nil {
			return errors.Wrap(err)
		}
	}
	if _, err := trans.Table(&models.MailboxState{}).Where("user_id=? and mailbox=?", ctx.UserID, oldFullPath).
		Update(map[string]interface{}{"mailbox": newFullPath}); err != nil {
		return errors.Wrap(err)
	}
	if err := trans.Commit(); err != nil {
		return errors.Wrap(err)
	}
	return nil
}

func GetGroupByName(ctx *context.Context, name string) (*models.Group, error) {
	var group models.Group
	db.Instance.Table("group").Where("name = ? and user_id = ?", name, ctx.UserID).Get(&group)

	return &group, nil
}

func GetGroupByFullPath(ctx *context.Context, fullPath string) (*models.Group, error) {
	var group models.Group
	_, err := db.Instance.Table("group").Where("full_path = ? and user_id = ?", fullPath, ctx.UserID).Get(&group)

	return &group, err
}

// ResolveBox finds a custom mailbox by its full path. Top-level names are also
// accepted for callers that only have a leaf name.
func ResolveBox(ctx *context.Context, nameOrPath string) (*models.Group, error) {
	nameOrPath = strings.Trim(strings.TrimSpace(nameOrPath), "/")
	if nameOrPath == "" {
		return &models.Group{}, nil
	}
	if group, err := GetGroupByFullPath(ctx, nameOrPath); err != nil || group.ID > 0 {
		return group, err
	}
	return GetGroupByName(ctx, nameOrPath)
}

func pathLeaf(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	return parts[len(parts)-1]
}

func pathParent(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) <= 1 {
		return ""
	}
	return strings.Join(parts[:len(parts)-1], "/")
}

func DelGroup(ctx *context.Context, groupId int) (bool, error) {
	trans := db.Instance.NewSession()
	defer trans.Close()
	if err := trans.Begin(); err != nil {
		return false, errors.Wrap(err)
	}
	defer trans.Rollback()

	var root models.Group
	found, err := trans.Where("id=? and user_id=?", groupId, ctx.UserID).Get(&root)
	if err != nil {
		return false, errors.Wrap(err)
	}
	if !found {
		return false, nil
	}

	// Only collect descendants owned by this user, in the same transaction.
	allGroupIds := []int{groupId}
	seen := map[int]bool{groupId: true}
	for pending := []int{groupId}; len(pending) > 0; {
		var children []models.Group
		if err := trans.Where("user_id=?", ctx.UserID).In("parent_id", pending).Find(&children); err != nil {
			return false, errors.Wrap(err)
		}
		pending = nil
		for _, child := range children {
			if !seen[child.ID] {
				seen[child.ID] = true
				allGroupIds = append(allGroupIds, child.ID)
				pending = append(pending, child.ID)
			}
		}
	}

	var removed []models.Group
	if err := trans.Where("user_id=?", ctx.UserID).In("id", allGroupIds).Find(&removed); err != nil {
		return false, errors.Wrap(err)
	}
	num, err := trans.Where("user_id=?", ctx.UserID).In("id", allGroupIds).Delete(&models.Group{})
	if err != nil {
		return false, errors.Wrap(err)
	}
	if len(removed) > 0 {
		paths := make([]string, 0, len(removed))
		for _, item := range removed {
			paths = append(paths, item.FullPath)
		}
		if _, err := trans.Table(&models.MailboxState{}).Where("user_id=?", ctx.UserID).In("mailbox", paths).Delete(&models.MailboxState{}); err != nil {
			return false, errors.Wrap(err)
		}
	}
	_, err = trans.Table(&models.UserEmail{}).Where("user_id=?", ctx.UserID).
		In("group_id", allGroupIds).Update(map[string]interface{}{"group_id": 0})
	if err != nil {
		return false, errors.Wrap(err)
	}
	if err := trans.Commit(); err != nil {
		return false, errors.Wrap(err)
	}
	return num > 0, nil
}

// GetGroupInfoList returns all groups
func GetGroupInfoList(ctx *context.Context) []*GroupItem {
	return buildChildren(ctx, 0)
}

// MoveMailToGroup moves an email into a group
func MoveMailToGroup(ctx *context.Context, mailId []int, groupId int) bool {
	if groupId > 0 {
		var group models.Group
		found, err := db.Instance.Where("id=? and user_id=?", groupId, ctx.UserID).Get(&group)
		if err != nil || !found {
			return false
		}
	}
	res, err := db.Instance.Exec(db.WithContext(ctx,
		fmt.Sprintf("update user_email set group_id=? where email_id in (%s) and user_id =?", array.Join(mailId, ","))),
		groupId, ctx.UserID)
	if err != nil {
		log.WithContext(ctx).Errorf("SQL Error:%+v", err)
		return false
	}
	rowNum, err := res.RowsAffected()
	if err != nil {
		log.WithContext(ctx).Errorf("SQL Error:%+v", err)
		return false
	}

	return rowNum > 0
}

func buildChildren(ctx *context.Context, parentId int) []*GroupItem {
	var ret []*GroupItem
	var rootGroup []*models.Group
	err := db.Instance.Table("group").Where("parent_id=? and user_id=?", parentId, ctx.UserID).Find(&rootGroup)
	if err != nil {
		log.WithContext(ctx).Errorf("SQL Error:%v", err)
	}

	for _, group := range rootGroup {
		ret = append(ret, &GroupItem{
			Id:       group.ID,
			Label:    group.Name,
			Tag:      dto.SearchTag{GroupId: group.ID, Status: -1, Type: -1}.ToString(),
			Unread:   unreadInGroup(ctx, group.ID),
			Children: buildChildren(ctx, group.ID),
		})
	}

	return ret
}

func GetGroupList(ctx *context.Context) []*models.Group {
	var ret []*models.Group
	db.Instance.Table("group").Where("user_id=?", ctx.UserID).Find(&ret)
	return ret
}

func getLayerName(ctx *context.Context, item *models.Group, allPath bool) string {
	if item.ParentId == 0 {
		return item.Name
	}
	var parent models.Group
	_, _ = db.Instance.Table("group").Where("id=? and user_id=?", item.ParentId, ctx.UserID).Get(&parent)
	if allPath {
		return getLayerName(ctx, &parent, allPath) + "/" + item.Name
	}
	return getLayerName(ctx, &parent, allPath)
}

func IsDefaultBox(box string) bool {
	return array.InArray(box, []string{"INBOX", "Sent Messages", "Drafts", "Deleted Messages", "Junk"})
}

func isReservedName(name string) bool {
	for reserved := range models.GroupNameToCode {
		if strings.EqualFold(name, reserved) {
			return true
		}
	}
	return false
}

func GetGroupStatus(ctx *context.Context, groupName string, params []string) (string, map[string]int) {
	retMap := map[string]int{}

	key := strings.Trim(strings.TrimSpace(groupName), "/")
	validity := models.GroupNameToCode[key]
	maxUID := 0
	groupID := 0
	if !IsDefaultBox(key) {
		box, err := ResolveBox(ctx, key)
		if err != nil || box == nil || box.ID == 0 {
			ret := ""
			for _, param := range params {
				if ret != "" {
					ret += " "
				}
				retMap[param] = 0
				ret += fmt.Sprintf("%s %d", param, 0)
			}
			return fmt.Sprintf("(%s)", ret), retMap
		}
		key = box.FullPath
		groupID = box.ID
		validity = box.ID
		maxUID = maxMailboxUID(ctx, box.ID)
	} else {
		maxUID = maxUIDForDefaultBox(ctx, key)
	}
	nextUID, validity := mailboxState(ctx, key, validity, maxUID)

	ret := ""
	for _, param := range params {
		if ret != "" {
			ret += " "
		}
		var value int

		switch param {
		case "MESSAGES":
			if groupID > 0 {
				db.Instance.Table("user_email").Select("count(1)").Where("user_id=? and group_id=?", ctx.UserID, groupID).Get(&value)
			} else {
				value = getGroupNum(ctx, key, false)
			}
		case "UIDNEXT":
			value = nextUID
		case "UIDVALIDITY":
			value = validity
		case "UNSEEN":
			if groupID > 0 {
				db.Instance.Table("user_email").Select("count(1)").Where("user_id=? and group_id=? and is_read=0", ctx.UserID, groupID).Get(&value)
			} else {
				value = getGroupNum(ctx, key, true)
			}
		default:
			continue
		}
		retMap[param] = value
		ret += fmt.Sprintf("%s %d", param, value)
	}
	if ret == "" {
		return "", retMap
	}

	return fmt.Sprintf("(%s)", ret), retMap
}

// mailboxState returns the UIDVALIDITY and a UIDNEXT that never decreases.
func mailboxState(ctx *context.Context, mailbox string, validity, maxUID int) (nextUID, uidValidity int) {
	var state models.MailboxState
	has, err := db.Instance.Where("user_id=? and mailbox=?", ctx.UserID, mailbox).Get(&state)
	if err != nil {
		log.WithContext(ctx).Errorf("Load mailbox state: %v", err)
		return maxUID + 1, validity
	}
	if !has {
		state = models.MailboxState{
			UserID:      ctx.UserID,
			Mailbox:     mailbox,
			UIDValidity: validity,
			UIDNext:     maxUID + 1,
		}
		if _, err := db.Instance.Insert(&state); err != nil {
			log.WithContext(ctx).Errorf("Insert mailbox state: %v", err)
		}
		return state.UIDNext, state.UIDValidity
	}
	next := state.UIDNext
	if next < 1 {
		next = 1
	}
	if maxUID+1 > next {
		next = maxUID + 1
	}
	if state.UIDValidity == 0 {
		state.UIDValidity = validity
	}
	if next != state.UIDNext || state.UIDValidity != validity {
		state.UIDNext = next
		if state.UIDValidity == 0 {
			state.UIDValidity = validity
		}
		if _, err := db.Instance.ID(state.ID).Cols("uid_next", "uid_validity").Update(&state); err != nil {
			log.WithContext(ctx).Errorf("Update mailbox state: %v", err)
		}
	}
	return state.UIDNext, state.UIDValidity
}

func maxMailboxUID(ctx *context.Context, groupID int) int {
	var lastID int
	db.Instance.Table("user_email").Select("id").Where("user_id=? and group_id=?", ctx.UserID, groupID).OrderBy("id desc").Get(&lastID)
	return lastID
}

func maxUIDForDefaultBox(ctx *context.Context, groupName string) int {
	var lastID int
	switch groupName {
	case "INBOX":
		db.Instance.Table("user_email").Select("id").Where("user_id=? and status = 0 and (group_id = 0 or group_id = ?)", ctx.UserID, models.INBOX).OrderBy("id desc").Get(&lastID)
	case "Sent Messages":
		db.Instance.Table("user_email").Select("id").Where("user_id=? and ((status = 1 and group_id = 0) or group_id = ?)", ctx.UserID, models.Sent).OrderBy("id desc").Get(&lastID)
	case "Drafts":
		db.Instance.Table("user_email").Select("id").Where("user_id=? and ((status = 4 and group_id = 0) or group_id = ?)", ctx.UserID, models.Drafts).OrderBy("id desc").Get(&lastID)
	case "Deleted Messages":
		db.Instance.Table("user_email").Select("id").Where("user_id=? and ((status = 3 and group_id = 0) or group_id = ?)", ctx.UserID, models.Deleted).OrderBy("id desc").Get(&lastID)
	case "Junk":
		db.Instance.Table("user_email").Select("id").Where("user_id=? and ((status = 5 and group_id = 0) or group_id = ?)", ctx.UserID, models.Junk).OrderBy("id desc").Get(&lastID)
	}
	return lastID
}

// unreadInGroup counts one user's unread copies inside a custom folder.
func unreadInGroup(ctx *context.Context, groupID int) int {
	var count int
	db.Instance.Table("user_email").Select("count(1)").
		Where("user_id=? and group_id=? and is_read=0", ctx.UserID, groupID).Get(&count)
	return count
}

// UnreadForTag returns the badge count a folder should show. Sent, drafts and
// deleted mail deliberately have no badge: an unread counter there is noise.
func UnreadForTag(ctx *context.Context, tag dto.SearchTag) int {
	if tag.GroupId > 0 {
		return unreadInGroup(ctx, tag.GroupId)
	}
	var count int
	switch {
	case tag.Type == 0 && tag.Status == -1: // INBOX
		db.Instance.Table("user_email").Select("count(1)").
			Where("user_id=? and status=0 and (group_id=0 or group_id=?) and is_read=0", ctx.UserID, models.INBOX).Get(&count)
	case tag.Status == 5: // Junk
		db.Instance.Table("user_email").Select("count(1)").
			Where("user_id=? and ((status=5 and group_id=0) or group_id=?) and is_read=0", ctx.UserID, models.Junk).Get(&count)
	}
	return count
}

func getGroupNum(ctx *context.Context, groupName string, mustUnread bool) int {
	var count int
	unread := ""
	if mustUnread {
		unread = " and is_read=0"
	}
	switch groupName {
	case "INBOX":
		db.Instance.Table("user_email").Select("count(1)").Where("user_id=? and status=0 and (group_id=0 or group_id=?)"+unread, ctx.UserID, models.INBOX).Get(&count)
	case "Sent Messages":
		if mustUnread {
			return 0
		}
		db.Instance.Table("user_email").Select("count(1)").Where("user_id=? and ((status=1 and group_id=0) or group_id=?)", ctx.UserID, models.Sent).Get(&count)
	case "Drafts":
		db.Instance.Table("user_email").Select("count(1)").Where("user_id=? and ((status=4 and group_id=0) or group_id=?)"+unread, ctx.UserID, models.Drafts).Get(&count)
	case "Deleted Messages":
		db.Instance.Table("user_email").Select("count(1)").Where("user_id=? and ((status=3 and group_id=0) or group_id=?)"+unread, ctx.UserID, models.Deleted).Get(&count)
	case "Junk":
		db.Instance.Table("user_email").Select("count(1)").Where("user_id=? and ((status=5 and group_id=0) or group_id=?)"+unread, ctx.UserID, models.Junk).Get(&count)
	}
	return count
}

func Move2DefaultBox(ctx *context.Context, mailIds []int, groupName string) error {
	switch groupName {
	case "Deleted Messages":
		err := del_email.DelEmail(ctx, mailIds, false)
		if err != nil {
			return err
		}
	case "INBOX":
		_, err := db.Instance.Table(&models.UserEmail{}).Where(builder.Eq{
			"user_id":  ctx.UserID,
			"email_id": mailIds,
		}).Update(map[string]interface{}{
			"status":   consts.EmailTypeReceive,
			"group_id": 0,
		})
		return err
	case "Sent Messages":
		_, err := db.Instance.Table(&models.UserEmail{}).Where(builder.Eq{
			"user_id":  ctx.UserID,
			"email_id": mailIds,
		}).Update(map[string]interface{}{
			"status":   consts.EmailStatusSent,
			"group_id": 0,
		})
		return err
	case "Drafts":
		_, err := db.Instance.Table(&models.UserEmail{}).Where(builder.Eq{
			"user_id":  ctx.UserID,
			"email_id": mailIds,
		}).Update(map[string]interface{}{
			"status":   consts.EmailStatusDrafts,
			"group_id": 0,
		})
		return err
	case "Junk":
		_, err := db.Instance.Table(&models.UserEmail{}).Where(builder.Eq{
			"user_id":  ctx.UserID,
			"email_id": mailIds,
		}).Update(map[string]interface{}{
			"status":   consts.EmailStatusJunk,
			"group_id": 0,
		})
		return err
	}
	return nil
}
