package audit

import "testing"

// The grading feature's mutating routes must stay mapped: the session-scores
// row is the ONLY evidence of who entered a component score (the owner may
// write on any teacher's behalf), so a missing entry would silently degrade the
// trail to "METHOD route".
func TestGradingRoutesAreRegistered(t *testing.T) {
	cases := []struct {
		method, route, action, entity, idParam string
	}{
		{"POST", "/api/v1/score-sets", "score_set.create", "score_set", ""},
		{"PUT", "/api/v1/score-sets/:id", "score_set.update", "score_set", "id"},
		{"DELETE", "/api/v1/score-sets/:id", "score_set.delete", "score_set", "id"},
		{"POST", "/api/v1/classes/:id/score-set", "class.score_set.assign", "class", "id"},
		{"DELETE", "/api/v1/classes/:id/score-set", "class.score_set.clear", "class", "id"},
		{"PUT", "/api/v1/sessions/:id/scores", "session.scores.update", "session", "id"},
	}
	for _, c := range cases {
		spec, ok := LookupAction(c.method, c.route)
		if !ok {
			t.Errorf("%s %s is not registered", c.method, c.route)
			continue
		}
		if spec.Action != c.action || spec.EntityType != c.entity || spec.IDParam != c.idParam {
			t.Errorf("%s %s mapped to %+v, want action=%q entity=%q idParam=%q",
				c.method, c.route, spec, c.action, c.entity, c.idParam)
		}
	}
}
