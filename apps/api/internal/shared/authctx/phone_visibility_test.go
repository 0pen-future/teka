package authctx

import "testing"

func TestPhoneVisible(t *testing.T) {
	owner := Scope{IsOwner: true}
	if !owner.PhoneVisible() {
		t.Error("owner must always see phones")
	}

	// Every member combination of holding reports.send and holding
	// contacts.view_all. The phone is visible exactly when the caller reads
	// contacts center-wide, directly or through reports.send; no class
	// assignment widens it.
	cases := []struct {
		name     string
		reports  bool
		contacts bool
		want     bool
	}{
		{"plain member", false, false, false},
		{"contacts.view_all", false, true, true},
		{"reports.send", true, false, true},
		{"both keys", true, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var grants []string
			if tc.reports {
				grants = append(grants, PermReportsSend)
			}
			if tc.contacts {
				grants = append(grants, PermContactsViewAll)
			}
			sc := Scope{Perms: BuildPermSet(nil, grants, nil)}
			if got := sc.PhoneVisible(); got != tc.want {
				t.Errorf("PhoneVisible() = %v, want %v", got, tc.want)
			}
		})
	}

	otherWide := Scope{Perms: BuildPermSet(nil, []string{PermStudentsViewAll}, nil)}
	if otherWide.PhoneVisible() {
		t.Error("another resource's view_all must not leak phones")
	}
}
