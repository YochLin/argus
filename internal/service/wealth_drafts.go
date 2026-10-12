package service

import (
	"encoding/json"
	"strings"

	"argus/internal/assets"
	"argus/internal/db"
)

// DraftCheck is a wealth assistant draft's computed state (Phase 28 §5.1):
// "ok" can be written, "pending" lacks Missing fields the user can still
// answer, "error" can't be written for the reason in Error. It is computed on
// every read, never stored, so an edit can't leave it stale.
type DraftCheck struct {
	Status  string   `json:"status"`
	Missing []string `json:"missing"`
	Error   string   `json:"error,omitempty"`
}

// CheckWealthDraft evaluates one draft against the records as they are now —
// existing is every non-archived asset, so a duplicate name or a balance
// update for an asset that has since gone away shows up as an error. Both
// the propose tools (before storing) and the web (for the drafts pane) call
// this, so the model and the user see the same verdict. A draft whose last
// write failed is an error with that reason until it is edited.
func CheckWealthDraft(d db.WealthDraft, existing []db.AssetWithValue, today string) DraftCheck {
	c := checkDraft(d.Kind, d.TargetID, d.Payload, existing, today)
	if d.ApplyError != "" {
		c.Status, c.Error = "error", d.ApplyError
	}
	return c
}

func checkDraft(kind string, targetID *int64, payload string, existing []db.AssetWithValue, today string) DraftCheck {
	var ps []assets.Problem
	switch kind {
	case assets.DraftAsset:
		var d assets.AssetDraft
		if err := json.Unmarshal([]byte(payload), &d); err != nil {
			return DraftCheck{Status: "error", Missing: []string{}, Error: "unreadable draft: " + err.Error()}
		}
		ps = d.Problems()
		key := assets.AssetKey(d.Name, d.Type)
		for _, a := range existing {
			if d.Name != "" && assets.AssetKey(a.Name, a.Type) == key {
				ps = append(ps, assets.Problem{Field: "name", Msg: "an asset with this name and type already exists — update its balance instead"})
				break
			}
		}
	case assets.DraftBalance:
		var d assets.BalanceDraft
		if err := json.Unmarshal([]byte(payload), &d); err != nil {
			return DraftCheck{Status: "error", Missing: []string{}, Error: "unreadable draft: " + err.Error()}
		}
		var id int64
		if targetID != nil {
			id = *targetID
		}
		ps = assets.ValidateBalance(id, d.Value, d.Date, today)
		if id > 0 && FindAsset(existing, id) == nil {
			ps = append(ps, assets.Problem{Field: "assetId", Msg: "the asset no longer exists"})
		}
	default:
		return DraftCheck{Status: "error", Missing: []string{}, Error: "unknown draft kind: " + kind}
	}

	c := DraftCheck{Status: "ok", Missing: []string{}}
	var errs []string
	for _, p := range ps {
		if p.Missing {
			c.Missing = append(c.Missing, p.Field)
		} else {
			errs = append(errs, p.Msg)
		}
	}
	switch {
	case len(errs) > 0:
		c.Status, c.Error = "error", strings.Join(errs, "; ")
	case len(c.Missing) > 0:
		c.Status = "pending"
	}
	return c
}

// FindAsset returns the asset with id from list, or nil.
func FindAsset(list []db.AssetWithValue, id int64) *db.AssetWithValue {
	for i := range list {
		if list[i].ID == id {
			return &list[i]
		}
	}
	return nil
}
