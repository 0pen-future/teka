package authctx

import "testing"

func TestPhoneVisible(t *testing.T) {
	owner := Scope{IsOwner: true}
	if !owner.PhoneVisible(false) {
		t.Error("owner must always see phones")
	}

	// Every member combination of: holding reports.send, holding
	// contacts.view_all, and the row itself being visible (assigned hoc_vu).
	// The phone is visible exactly when the caller reads contacts
	// center-wide (directly or through reports.send) or owns the row.
	cases := []struct {
		name       string
		reports    bool
		contacts   bool
		rowVisible bool
		want       bool
	}{
		{"plain member, foreign row", false, false, false, false},
		{"plain member, own row", false, false, true, true},
		{"contacts.view_all, foreign row", false, true, false, true},
		{"contacts.view_all, own row", false, true, true, true},
		{"reports.send, foreign row", true, false, false, true},
		{"reports.send, own row", true, false, true, true},
		{"both keys, foreign row", true, true, false, true},
		{"both keys, own row", true, true, true, true},
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
			perms := BuildPermSet(nil, grants, nil)
			sc := Scope{CanSendReports: perms.HasKey(PermReportsSend), Perms: perms}
			if got := sc.PhoneVisible(tc.rowVisible); got != tc.want {
				t.Errorf("PhoneVisible(%v) = %v, want %v", tc.rowVisible, got, tc.want)
			}
		})
	}

	otherWide := Scope{Perms: BuildPermSet(nil, []string{PermStudentsViewAll}, nil)}
	if otherWide.PhoneVisible(false) {
		t.Error("another resource's view_all must not leak phones")
	}

	// A scope whose CanSendReports flag is set without the key in Perms is
	// not a resolvable state (every resolver derives the flag from the key);
	// the phone rule reads the key, so such a scope sees nothing extra.
	stale := Scope{CanSendReports: true}
	if stale.PhoneVisible(false) {
		t.Error("phone visibility must follow the resolved key set, not the bare flag")
	}
}
