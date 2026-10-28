package schema

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/kubesense-ai/terraform-provider-kubesense/internal/kschema/presetwalk"
)

// This file owns every rule about a preset's tabs, including the ones about a
// single field. The CUE #Tab does not restate them: a shape finding is only
// logged on the UI's save path, and each rule here must refuse that save too.
//
// Two tabs sharing an id make the ?tab= URL and every tab-addressed edit
// ambiguous, two sharing a title are indistinguishable on the tab bar, stray
// top-level panels on a tabbed dashboard render nowhere, and a tab panel with
// no layout cell has nowhere to render. Tabs postdate the contract, so no
// stored dashboard breaks these rules and refusing them blocks nothing saved.

var tabIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)

const maxTabTitleRunes = 50

// refusedForEveryOrigin names the rules no save may log and proceed past,
// whoever sends it.
var refusedForEveryOrigin = map[string]bool{
	"tabs_exclusive":  true,
	"tab_id":          true,
	"tab_title":       true,
	"sub_grid_id":     true,
	"tab_grid_layout": true,
}

// RefusedForEveryOrigin returns the findings a write path refuses even where
// it only logs the rest.
func (errs Errors) RefusedForEveryOrigin() Errors {
	var out Errors
	for _, e := range errs {
		if refusedForEveryOrigin[e.Rule] {
			out = append(out, e)
		}
	}
	return out
}

// checkTabs applies the tab rules. Every member is type-asserted, so it runs
// on a preset that failed shape validation as well as on a sound one.
func checkTabs(doc map[string]any) Errors {
	if !presetwalk.HasTabs(doc) {
		return nil
	}

	var errs Errors
	for _, key := range []string{"panels", "gridLayout", "subGrids", "subGridLayout"} {
		if len(list(doc[key])) > 0 {
			errs = append(errs, Error{
				Path:    "/" + key,
				Message: "must be empty when the dashboard has tabs — move its contents into a tab",
				Rule:    "tabs_exclusive",
			})
		}
	}

	ids, titles, subGrids := firstSeen{}, firstSeen{}, firstSeen{}
	for _, page := range presetwalk.Pages(doc) {
		if page.IsTab() {
			errs = append(errs, checkTab(page, ids, titles)...)
		}
		for k, entry := range list(page.Object["subGrids"]) {
			id := text(object(entry)["id"])
			if id == "" {
				continue
			}
			path := fmt.Sprintf("%s/subGrids/%d/id", page.Pointer, k)
			if first, dup := subGrids.claim(id, path); dup {
				errs = append(errs, Error{
					Path:    path,
					Message: fmt.Sprintf("sub-grid id %q is already used at %s — sub-grid ids must be unique across every tab", id, first),
					Rule:    "sub_grid_id",
				})
			}
		}
	}
	return errs
}

// checkTab applies the rules about one tab, recording its id and title for the
// uniqueness rules.
func checkTab(page presetwalk.Page, ids, titles firstSeen) Errors {
	var errs Errors
	at := page.Pointer

	if !tabIDPattern.MatchString(page.TabID) {
		errs = append(errs, Error{
			Path:    at + "/id",
			Message: fmt.Sprintf("tab id %q must be 1 to 40 lowercase letters, digits or dashes, starting with a letter or digit", page.TabID),
			Rule:    "tab_id",
		})
	} else if first, dup := ids.claim(page.TabID, at); dup {
		errs = append(errs, Error{
			Path:    at + "/id",
			Message: fmt.Sprintf("tab id %q is already used by %s — tab ids must be unique", page.TabID, first),
			Rule:    "tab_id",
		})
	}

	title := strings.TrimSpace(page.TabTitle)
	switch runes := utf8.RuneCountInString(title); {
	case runes == 0:
		errs = append(errs, Error{Path: at + "/title", Message: "tab title must not be empty", Rule: "tab_title"})
	case runes > maxTabTitleRunes:
		errs = append(errs, Error{
			Path:    at + "/title",
			Message: fmt.Sprintf("tab title is %d characters; the limit is %d", runes, maxTabTitleRunes),
			Rule:    "tab_title",
		})
	default:
		if first, dup := titles.claim(strings.ToLower(title), at); dup {
			errs = append(errs, Error{
				Path:    at + "/title",
				Message: fmt.Sprintf("tab title %q is already used by %s — titles must be unique, ignoring case", title, first),
				Rule:    "tab_title",
			})
		}
	}

	// The webapp pairs panels with layout cells by index, so each tab needs
	// exactly one cell per panel. The untabbed top level is not held to this:
	// stored and fixture presets carry short top-level layouts, and the agent
	// tools check that one at their own boundary.
	if panels, layout := len(list(page.Object["panels"])), len(list(page.Object["gridLayout"])); panels != layout {
		errs = append(errs, Error{
			Path:    at + "/gridLayout",
			Message: fmt.Sprintf("has %d layout cell(s) for %d panel(s) — each panel needs exactly one cell, matched by position", layout, panels),
			Rule:    "tab_grid_layout",
		})
	}
	return errs
}

// firstSeen maps a key to the path it was first seen at.
type firstSeen map[string]string

// claim records path for key and returns where key was first seen, if it was.
func (s firstSeen) claim(key, path string) (string, bool) {
	if first, ok := s[key]; ok {
		return first, true
	}
	s[key] = path
	return "", false
}
