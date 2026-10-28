// Package presetwalk finds the panels of a decoded dashboard preset.
//
// A preset holds its panels on pages. A page is {panels, gridLayout, subGrids,
// subGridLayout}: the untabbed top level is one page, and each entry of `tabs`
// carries its own. Within a page, panels sit either on the page's own grid or
// inside one of its sub-grids, one level deep.
//
// Every reader that needs "all the panels" goes through here, so a new place a
// panel can live is added once rather than at each walker. The walk is literal
// rather than recursive: a recursive one would descend into panel config and
// query filters, where a stray "panels" key would be taken for a sub-grid's.
//
// Every step is type-asserted and skipped on mismatch. Callers include the
// legacy migration, which runs over documents nothing has validated.
package presetwalk

import "fmt"

// Page is one panel surface of a preset.
type Page struct {
	// TabID and TabTitle are empty for the top level.
	TabID    string
	TabTitle string
	// Pointer is the JSON Pointer to Object: "" for the top level, "/tabs/<i>"
	// for a tab.
	Pointer string
	// Object holds the page's panels, gridLayout, subGrids and subGridLayout.
	Object map[string]any
}

// IsTab reports whether the page is a tab rather than the top level.
func (p Page) IsTab() bool { return p.Pointer != "" }

// Pages returns the top level, then every tab that is an object, in document
// order. The top level is included on a tabbed preset too, where it should be
// empty, so a preset that breaks that rule still has its stray panels found.
func Pages(doc map[string]any) []Page {
	pages := []Page{{Object: doc}}
	tabs, _ := doc["tabs"].([]any)
	for i, entry := range tabs {
		tab, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		id, _ := tab["id"].(string)
		title, _ := tab["title"].(string)
		pages = append(pages, Page{
			TabID:    id,
			TabTitle: title,
			Pointer:  fmt.Sprintf("/tabs/%d", i),
			Object:   tab,
		})
	}
	return pages
}

// HasTabs reports whether the preset is tabbed: `tabs` is a non-empty array.
func HasTabs(doc map[string]any) bool {
	tabs, _ := doc["tabs"].([]any)
	return len(tabs) > 0
}

// Panel is one panel object and where it sits.
type Panel struct {
	Page Page
	// SubGrid is the sub-grid holding the panel, nil for a panel on the page's
	// own grid.
	SubGrid map[string]any
	// Pointer is the JSON Pointer to the panel.
	Pointer string
	// Position is the panel's index in the panels array that holds it.
	Position int
	Object   map[string]any
}

// Panels returns every panel object in the preset, page by page. Within a page
// the page's own panels come first, then each sub-grid's.
func Panels(doc map[string]any) []Panel {
	var out []Panel
	for _, page := range Pages(doc) {
		collect := func(holder map[string]any, subGrid map[string]any, pointer string) {
			panels, _ := holder["panels"].([]any)
			for i, entry := range panels {
				if panel, ok := entry.(map[string]any); ok {
					out = append(out, Panel{
						Page:     page,
						SubGrid:  subGrid,
						Pointer:  fmt.Sprintf("%s/panels/%d", pointer, i),
						Position: i,
						Object:   panel,
					})
				}
			}
		}
		collect(page.Object, nil, page.Pointer)
		subGrids, _ := page.Object["subGrids"].([]any)
		for k, entry := range subGrids {
			if subGrid, ok := entry.(map[string]any); ok {
				collect(subGrid, subGrid, fmt.Sprintf("%s/subGrids/%d", page.Pointer, k))
			}
		}
	}
	return out
}
