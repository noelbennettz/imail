package ssl

import (
	"fmt"
	"testing"

	"github.com/noelbennettz/imail/config"
)

func TestCheckSSLCrtInfo(t *testing.T) {
	config.Init()

	got, got1, match, err := CheckSSLCrtInfo()

	fmt.Println(got, got1, match, err)
}
