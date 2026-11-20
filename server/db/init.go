package db

import (
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/models"
	"github.com/noelbennettz/imail/utils/context"
	"github.com/noelbennettz/imail/utils/errors"
	log "github.com/sirupsen/logrus"
	"xorm.io/xorm"
)

var Instance *xorm.Engine

func Init(version string) error {
	dsn := config.Instance.DbDSN
	var err error
	if config.Instance.DbType == config.DBTypeSQLite {
		dsn, err = sqliteDSNWithReliabilityOptions(dsn)
		if err != nil {
			return errors.Wrap(err)
		}
	}

	switch config.Instance.DbType {
	case "mysql":
		Instance, err = xorm.NewEngine("mysql", dsn)
	case "sqlite":
		Instance, err = xorm.NewEngine("sqlite", dsn)
	case "postgres":
		Instance, err = xorm.NewEngine("postgres", dsn)
	default:
		return errors.New("Database Type Error!")
	}
	if err != nil {
		log.Errorf("DB init Error! %s", err.Error())
		return errors.Wrap(err)
	}
	if config.Instance.DbType == config.DBTypeSQLite {
		Instance.SetMaxOpenConns(1)
		Instance.SetMaxIdleConns(1)
	} else {
		Instance.SetMaxOpenConns(100)
		Instance.SetMaxIdleConns(10)
	}

	Instance.SetConnMaxLifetime(30 * time.Minute)
	Instance.ShowSQL(config.Instance.LogLevel == "debug")
	// Sync table schemas
	syncTables()

	// Migrate historical data
	fixHistoryData()

	// Record the program version in the database
	var v models.Version
	_, err = Instance.Get(&v)
	if err != nil {
		panic(err)
	}

	if version != "" && v.Info != version && version != "test" {
		v.Info = version
		if v.Id == 0 {
			Instance.Insert(&v)
		} else {
			Instance.Update(&v)
		}
	}

	//if config.Instance.LogLevel == "debug" {
	//	Instance.ShowSQL(true)
	//}

	return nil
}

func WithContext(ctx *context.Context, sql string) string {
	if ctx != nil {
		logId := ctx.GetValue(context.LogID)
		return fmt.Sprintf("/* %s */ %s", logId, sql)
	}
	return sql
}

func syncTables() {
	err := Instance.Sync2(&models.User{})
	if err != nil {
		panic(err)
	}
	err = Instance.Sync2(&models.Email{})
	if err != nil {
		panic(err)
	}
	err = Instance.Sync2(&models.Group{})
	if err != nil {
		panic(err)
	}
	err = Instance.Sync2(&models.Rule{})
	if err != nil {
		panic(err)
	}
	err = Instance.Sync2(&models.Sessions{})
	if err != nil {
		panic(err)
	}
	err = Instance.Sync2(&models.UserEmail{})
	if err != nil {
		panic(err)
	}
	err = Instance.Sync2(&models.Version{})
	if err != nil {
		panic(err)
	}
	err = Instance.Sync2(&models.EmailQueue{})
	if err != nil {
		panic(err)
	}
	err = Instance.Sync2(&models.MailboxState{})
	if err != nil {
		panic(err)
	}
}

func fixHistoryData() {
	var ueNum int
	_, err := Instance.Table(&models.UserEmail{}).Select("count(1)").Get(&ueNum)
	if err != nil {
		panic(err)
	}
	if ueNum > 0 {
		return
	}

	// Only one admin user exists
	var user []models.User
	err = Instance.Table(&models.User{}).OrderBy("id asc").Find(&user)
	if err != nil {
		panic(err)
	}

	// Only one account exists and it is not an admin: promote it to admin
	if len(user) == 1 && user[0].IsAdmin == 0 {
		u := user[0]
		u.IsAdmin = 1
		_, err = Instance.Update(&u)
		if err != nil {
			panic(err)
		}
	}

	if len(user) != 1 {
		return
	}

	// Emails exist from before
	var emails []*models.Email
	err = Instance.Table(&models.Email{}).Select("id,status").OrderBy("id asc").Find(&emails)
	if err != nil {
		panic(err)
	}
	if len(emails) == 0 {
		return
	}

	log.Infof("Sync History Data！Please Wait！")

	// Assign all pre-existing emails to the admin account
	for _, email := range emails {
		ue := models.UserEmail{
			UserID:  user[0].ID,
			EmailID: email.Id,
			Status:  email.Status,
		}
		_, err = Instance.Insert(&ue)
		if err != nil {
			log.Errorf("SQL Error: %v", err)
		}
	}
	log.Infof("Sync History Data Finished. Num: %d", len(emails))
}
