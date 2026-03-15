package web

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/krystophny/tabura/internal/store"
)

func latestUserMessage(messages []store.ChatMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if !strings.EqualFold(strings.TrimSpace(messages[i].Role), "user") {
			continue
		}
		text := strings.TrimSpace(messages[i].ContentPlain)
		if text == "" {
			text = strings.TrimSpace(messages[i].ContentMarkdown)
		}
		if text != "" {
			return text
		}
	}
	return ""
}

func queuedUserMessage(messages []store.ChatMessage, messageID int64) string {
	if messageID > 0 {
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].ID != messageID {
				continue
			}
			text := strings.TrimSpace(messages[i].ContentPlain)
			if text == "" {
				text = strings.TrimSpace(messages[i].ContentMarkdown)
			}
			if text != "" {
				return text
			}
			break
		}
	}
	return latestUserMessage(messages)
}

func (a *App) preferredProject() (store.Project, error) {
	activeID, err := a.store.ActiveWorkspaceID()
	if err == nil && strings.TrimSpace(activeID) != "" {
		if project, getErr := a.store.GetProject(activeID); getErr == nil {
			return project, nil
		}
	}
	defaultProject, err := a.ensureDefaultProjectRecord()
	if err == nil {
		return defaultProject, nil
	}
	projects, listErr := a.store.ListProjects()
	if listErr != nil {
		return store.Project{}, listErr
	}
	if len(projects) == 0 {
		return store.Project{}, errors.New("no project is available")
	}
	return projects[0], nil
}

func (a *App) findProjectByName(name string) (store.Project, error) {
	query := strings.ToLower(strings.TrimSpace(name))
	if query == "" {
		return store.Project{}, errors.New("project name is required")
	}
	projects, err := a.store.ListProjects()
	if err != nil {
		return store.Project{}, err
	}
	exact := make([]store.Project, 0, 2)
	partial := make([]store.Project, 0, 4)
	for _, project := range projects {
		candidate := strings.ToLower(strings.TrimSpace(project.Name))
		if candidate == "" {
			continue
		}
		if candidate == query {
			exact = append(exact, project)
			continue
		}
		if strings.Contains(candidate, query) {
			partial = append(partial, project)
		}
	}
	if len(exact) > 0 {
		sort.Slice(exact, func(i, j int) bool {
			return len(exact[i].Name) < len(exact[j].Name)
		})
		return exact[0], nil
	}
	if len(partial) > 0 {
		sort.Slice(partial, func(i, j int) bool {
			return len(partial[i].Name) < len(partial[j].Name)
		})
		return partial[0], nil
	}
	return store.Project{}, fmt.Errorf("project %q was not found", name)
}
