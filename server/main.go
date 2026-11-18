package main

import (
	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/listen/cron_server"
	"github.com/noelbennettz/imail/res_init"
	log "github.com/sirupsen/logrus"
)

var (
	gitHash   string
	buildTime string
	goVersion string
	version   string
)

func main() {

	config.Init()

	if version == "" {
		version = "TestVersion"
	}

	log.Infoln("*******************************************************************")
	log.Infof("***\tServer Start Success \n")
	log.Infof("***\tServer Version: %s \n", version)
	log.Infof("***\tGit Commit Hash: %s ", gitHash)
	log.Infof("***\tBuild Date: %s ", buildTime)
	log.Infof("***\tBuild GoLang Version: %s ", goVersion)
	log.Infoln("*******************************************************************")

	// Start the scheduled tasks
	go cron_server.Start()

	// Start the core services
	res_init.Init(version)

	log.Warnf("Server Stoped \n")

}
