package setup

import (
	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/signal"
	"github.com/noelbennettz/imail/utils/errors"
)

// Finish marks initialization as complete
func Finish() error {
	cfg, err := config.ReadConfig()
	if err != nil {
		return errors.Wrap(err)
	}
	cfg.IsInit = true

	err = config.WriteConfig(cfg)
	if err != nil {
		return errors.Wrap(err)
	}
	// The setup token has done its job: drop it from memory and close the setup
	// API for good.
	ClearToken()
	// Initialization complete
	signal.InitChan <- true
	return nil
}
