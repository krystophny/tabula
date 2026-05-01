package web

import (
	"net/http"

	"github.com/sloppy-org/slopshell/internal/store"
)

// handleItemProjectReview answers the GTD composite-outcome review surface.
//
// The response lists every active Item(kind=project) — workspace records and
// external source containers (Todoist projects, GitHub Projects, mail folders)
// are intentionally absent. Each row carries the project item's current health
// flags and per-state child counts so the weekly review can spot stalled
// outcomes without inventing tasks.
func (a *App) handleItemProjectReview(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	if !a.resurfaceDueItemsForRead(w) {
		return
	}
	filter, err := parseItemListFilterQuery(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	reviews, err := a.store.ListProjectItemReviewsFiltered(filter)
	if err != nil {
		writeItemStoreError(w, err)
		return
	}
	stalled := countStalledProjectItems(reviews)
	writeAPIData(w, http.StatusOK, map[string]any{
		"project_items": reviews,
		"total":         len(reviews),
		"stalled":       stalled,
	})
}

func (a *App) handleItemPeopleDashboard(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	if !a.resurfaceDueItemsForRead(w) {
		return
	}
	filter, err := parseItemListFilterQuery(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	people, err := a.store.ListPersonOpenLoopDashboardsFiltered(filter)
	if err != nil {
		writeItemStoreError(w, err)
		return
	}
	writeAPIData(w, http.StatusOK, map[string]any{
		"people": people,
		"total":  len(people),
	})
}

func (a *App) handleItemPersonDashboard(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	if !a.resurfaceDueItemsForRead(w) {
		return
	}
	actorID, err := parseURLInt64Param(r, "actor_id")
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	filter, err := parseItemListFilterQuery(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	person, err := a.store.GetPersonOpenLoopDashboardFiltered(actorID, filter)
	if err != nil {
		writeItemStoreError(w, err)
		return
	}
	writeAPIData(w, http.StatusOK, map[string]any{"person": person})
}

func countStalledProjectItems(reviews []store.ProjectItemReview) int {
	stalled := 0
	for _, review := range reviews {
		if review.Health.Stalled {
			stalled++
		}
	}
	return stalled
}
