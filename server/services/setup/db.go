package setup

import (
	"strings"

	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/db"
	"github.com/noelbennettz/imail/models"
	"github.com/noelbennettz/imail/utils/array"
	"github.com/noelbennettz/imail/utils/context"
	"github.com/noelbennettz/imail/utils/errors"
	"github.com/noelbennettz/imail/utils/password"
)

func GetDatabaseSettings(ctx *context.Context) (string, string, error) {
	configData, err := config.ReadConfig()
	if err != nil {
		return "", "", errors.Wrap(err)
	}

	if configData.DbType == "" && configData.DbDSN == "" {
		return config.DBTypeSQLite, config.ROOT_PATH + "./config/imail.db", nil
	}

	return configData.DbType, configData.DbDSN, nil
}

func GetAdminPassword(ctx *context.Context) (string, error) {
	users := []*models.User{}
	err := db.Instance.Find(&users)
	if err != nil {
		return "", errors.Wrap(err)
	}

	if len(users) > 0 {
		return users[0].Account, nil
	}

	return "", nil
}

func SetAdminPassword(ctx *context.Context, account, pwd string) error {
	account = strings.TrimSpace(account)
	if account == "" || len(pwd) < 8 {
		return errors.New("account and a password of at least 8 characters are required")
	}
	if strings.ContainsAny(account, "@ \t\r\n/\\") {
		return errors.New("account must not contain @ or whitespace")
	}
	encodePwd := password.Encode(pwd)

	// The wizard can be retried after a lost response. Update the account that
	// was already created instead of inserting a second administrator.
	var users []models.User
	if err := db.Instance.OrderBy("id asc").Find(&users); err != nil {
		return errors.Wrap(err)
	}
	if len(users) > 0 {
		target := users[0]
		_, err := db.Instance.Table(&models.User{}).Where("id=?", target.ID).Cols("account", "name", "password", "is_admin").Update(map[string]interface{}{
			"account":  account,
			"name":     "admin",
			"password": encodePwd,
			"is_admin": 1,
		})
		if err != nil {
			return errors.Wrap(err)
		}
		return nil
	}

	user := models.User{
		Account:  account,
		Name:     "admin",
		Password: encodePwd,
		IsAdmin:  1,
	}
	_, err := db.Instance.Insert(&user)
	if err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func SetDatabaseSettings(ctx *context.Context, dbType, dbDSN string) error {
	configData, err := config.ReadConfig()
	if err != nil {
		return errors.Wrap(err)
	}

	if !array.InArray(dbType, config.DBTypes) {
		return errors.New("dbtype error")
	}

	if dbDSN == "" {
		return errors.New("DSN error")
	}

	configData.DbType = dbType
	configData.DbDSN = dbDSN

	err = config.WriteConfig(configData)
	if err != nil {
		return errors.Wrap(err)
	}
	config.Init()
	// Check whether the database can be reached
	err = db.Init("")
	if err != nil {
		return errors.Wrap(err)
	}
	return nil
}
