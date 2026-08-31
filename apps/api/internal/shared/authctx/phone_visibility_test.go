package authctx

import "testing"

func TestPhoneVisible(t *testing.T) {
	owner := Scope{IsOwner: true}
	if !owner.PhoneVisible(false) {
		t.Error("owner must always see phones")
	}

	secretary := Scope{CanSendReports: true}
	if !secretary.PhoneVisible(false) {
		t.Error("reports oversight must see phones regardless of row visibility")
	}

	viewer := Scope{Perms: BuildPermSet(nil, []string{PermContactsViewAll}, nil)}
	if !viewer.PhoneVisible(false) {
		t.Error("contacts.view_all must see phones — a contact row IS its phone")
	}

	otherWide := Scope{Perms: BuildPermSet(nil, []string{PermStudentsViewAll}, nil)}
	if otherWide.PhoneVisible(false) {
		t.Error("another resource's view_all must not leak phones")
	}

	var member Scope
	if member.PhoneVisible(false) {
		t.Error("plain member must not see phones without row visibility")
	}
	if !member.PhoneVisible(true) {
		t.Error("row-level visibility (assigned hoc_vu) must grant the phone")
	}
}
