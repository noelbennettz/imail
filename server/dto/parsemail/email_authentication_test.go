package parsemail

import (
	"testing"

	"github.com/noelbennettz/imail/models"
)

func TestNewEmailAuthentication(t *testing.T) {
	tests := []struct {
		name       string
		spfPassed  bool
		dkimPassed bool
		dangerous  bool
	}{
		{name: "both SPF and DKIM pass", spfPassed: true, dkimPassed: true},
		{name: "only SPF passes", spfPassed: true, dkimPassed: false},
		{name: "only DKIM passes", spfPassed: false, dkimPassed: true},
		{name: "neither SPF nor DKIM passes", spfPassed: false, dkimPassed: false, dangerous: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authentication := NewEmailAuthentication(tt.spfPassed, tt.dkimPassed)
			if authentication.SPFPassed != tt.spfPassed {
				t.Fatalf("SPFPassed = %v, want %v", authentication.SPFPassed, tt.spfPassed)
			}
			if authentication.DKIMPassed != tt.dkimPassed {
				t.Fatalf("DKIMPassed = %v, want %v", authentication.DKIMPassed, tt.dkimPassed)
			}
			if authentication.Dangerous != tt.dangerous {
				t.Fatalf("Dangerous = %v, want %v", authentication.Dangerous, tt.dangerous)
			}
		})
	}
}

func TestNewEmailFromModelRestoresAuthenticationForReceivedEmail(t *testing.T) {
	email := NewEmailFromModel(models.Email{
		Type:      0,
		SPFCheck:  1,
		DKIMCheck: 0,
	})

	if email.Authentication == nil {
		t.Fatal("received email is missing the authentication result")
	}
	if !email.Authentication.SPFPassed || email.Authentication.DKIMPassed || email.Authentication.Dangerous {
		t.Fatalf("incorrect authentication result: %+v", email.Authentication)
	}
}

func TestNewEmailFromModelLeavesAuthenticationNilForSentEmail(t *testing.T) {
	email := NewEmailFromModel(models.Email{
		Type:      1,
		SPFCheck:  1,
		DKIMCheck: 1,
	})

	if email.Authentication != nil {
		t.Fatalf("outgoing email should not carry an incoming authentication result: %+v", email.Authentication)
	}
}
