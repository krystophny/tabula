package web

import "github.com/krystophny/tabura/internal/store"

func isExplicitWorkspace(_ store.Workspace) bool {
	return true
}

func filterExplicitWorkspaces(workspaces []store.Workspace) []store.Workspace {
	return workspaces
}

func activeExplicitWorkspace(workspaces []store.Workspace) *store.Workspace {
	for i := range workspaces {
		if workspaces[i].IsActive {
			return &workspaces[i]
		}
	}
	return nil
}
