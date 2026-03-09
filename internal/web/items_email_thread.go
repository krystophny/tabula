package web

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/krystophny/tabura/internal/providerdata"
	"github.com/krystophny/tabura/internal/store"
	tabsync "github.com/krystophny/tabura/internal/sync"
)

const emailThreadBindingObjectType = "email_thread"

type emailPersistedMessage struct {
	Message      *providerdata.EmailMessage
	Artifact     store.Artifact
	ItemID       *int64
	FollowUpItem bool
}

type emailThreadRecord struct {
	ThreadID string
	Artifact store.Artifact
	Messages []emailPersistedMessage
}

func emailThreadIDForMessage(message *providerdata.EmailMessage) string {
	if message == nil {
		return ""
	}
	threadID := strings.TrimSpace(message.ThreadID)
	if threadID != "" {
		return threadID
	}
	return strings.TrimSpace(message.ID)
}

func emailThreadTitle(messages []emailPersistedMessage) string {
	for _, message := range sortEmailMessagesByDate(messages) {
		subject := cleanEmailThreadSubject(message.Message)
		if subject != "" {
			return subject
		}
	}
	if len(messages) > 0 {
		if sender := strings.TrimSpace(messages[0].Message.Sender); sender != "" {
			return sender
		}
	}
	return "Email thread"
}

func cleanEmailThreadSubject(message *providerdata.EmailMessage) string {
	if message == nil {
		return ""
	}
	subject := strings.TrimSpace(message.Subject)
	for subject != "" {
		lower := strings.ToLower(subject)
		switch {
		case strings.HasPrefix(lower, "re:"):
			subject = strings.TrimSpace(subject[3:])
		case strings.HasPrefix(lower, "fw:"):
			subject = strings.TrimSpace(subject[3:])
		case strings.HasPrefix(lower, "fwd:"):
			subject = strings.TrimSpace(subject[4:])
		default:
			return subject
		}
	}
	return ""
}

func sortEmailMessagesByDate(messages []emailPersistedMessage) []emailPersistedMessage {
	out := append([]emailPersistedMessage(nil), messages...)
	sort.Slice(out, func(i, j int) bool {
		left := time.Time{}
		right := time.Time{}
		if out[i].Message != nil {
			left = out[i].Message.Date
		}
		if out[j].Message != nil {
			right = out[j].Message.Date
		}
		switch {
		case left.Equal(right):
			leftID := ""
			rightID := ""
			if out[i].Message != nil {
				leftID = strings.TrimSpace(out[i].Message.ID)
			}
			if out[j].Message != nil {
				rightID = strings.TrimSpace(out[j].Message.ID)
			}
			return leftID < rightID
		case left.IsZero():
			return false
		case right.IsZero():
			return true
		default:
			return left.After(right)
		}
	})
	return out
}

func emailThreadParticipants(messages []emailPersistedMessage) []string {
	seen := make(map[string]string)
	for _, persisted := range messages {
		if persisted.Message == nil {
			continue
		}
		for _, raw := range append([]string{persisted.Message.Sender}, persisted.Message.Recipients...) {
			participant := strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
			if participant == "" {
				continue
			}
			key := strings.ToLower(participant)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = participant
		}
	}
	out := make([]string, 0, len(seen))
	for _, participant := range seen {
		out = append(out, participant)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i]) < strings.ToLower(out[j])
	})
	return out
}

func emailThreadRemoteUpdatedAt(messages []emailPersistedMessage) *string {
	for _, persisted := range sortEmailMessagesByDate(messages) {
		if persisted.Message == nil || persisted.Message.Date.IsZero() {
			continue
		}
		value := persisted.Message.Date.UTC().Format(time.RFC3339)
		return &value
	}
	return nil
}

func emailThreadContainerRef(messages []emailPersistedMessage) *string {
	for _, persisted := range sortEmailMessagesByDate(messages) {
		if ref := emailMessageContainerRef(persisted.Message); ref != nil {
			return ref
		}
	}
	return nil
}

func emailThreadMetaJSON(threadID string, messages []emailPersistedMessage) (string, error) {
	payload := map[string]any{
		"thread_id":     strings.TrimSpace(threadID),
		"message_count": len(messages),
		"participants":  emailThreadParticipants(messages),
		"subject":       emailThreadTitle(messages),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func (a *App) persistEmailThreads(ctx context.Context, sink tabsync.Sink, account store.ExternalAccount, messages []emailPersistedMessage) ([]emailThreadRecord, error) {
	grouped := make(map[string][]emailPersistedMessage)
	for _, persisted := range messages {
		threadID := emailThreadIDForMessage(persisted.Message)
		if threadID == "" {
			continue
		}
		grouped[threadID] = append(grouped[threadID], persisted)
	}
	if len(grouped) == 0 {
		return nil, nil
	}

	threadIDs := make([]string, 0, len(grouped))
	for threadID := range grouped {
		threadIDs = append(threadIDs, threadID)
	}
	sort.Strings(threadIDs)

	out := make([]emailThreadRecord, 0, len(threadIDs))
	for _, threadID := range threadIDs {
		group := grouped[threadID]
		title := emailThreadTitle(group)
		metaJSON, err := emailThreadMetaJSON(threadID, group)
		if err != nil {
			return nil, err
		}
		artifact, err := sink.UpsertArtifact(ctx, store.Artifact{
			Kind:     store.ArtifactKindEmailThread,
			Title:    &title,
			MetaJSON: &metaJSON,
		}, store.ExternalBinding{
			AccountID:       account.ID,
			Provider:        account.Provider,
			ObjectType:      emailThreadBindingObjectType,
			RemoteID:        threadID,
			ContainerRef:    emailThreadContainerRef(group),
			RemoteUpdatedAt: emailThreadRemoteUpdatedAt(group),
		})
		if err != nil {
			return nil, err
		}
		for _, persisted := range group {
			if persisted.ItemID == nil {
				continue
			}
			if err := a.store.LinkItemArtifact(*persisted.ItemID, artifact.ID, "related"); err != nil {
				return nil, err
			}
		}
		out = append(out, emailThreadRecord{
			ThreadID: threadID,
			Artifact: artifact,
			Messages: sortEmailMessagesByDate(group),
		})
	}
	return out, nil
}
