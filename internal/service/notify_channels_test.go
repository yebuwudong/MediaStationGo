package service

import (
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestChannelSubscribesCanDisableAllEvents(t *testing.T) {
	channel := model.NotifyChannel{Events: `["` + NotifyEventNone + `"]`}

	if channelSubscribes(channel, EventDownloadComplete) {
		t.Fatal("explicit none sentinel should disable event pushes")
	}
}

func TestChannelSubscribesKeepsLegacyEmptyAsAllEvents(t *testing.T) {
	for _, raw := range []string{"", "[]"} {
		channel := model.NotifyChannel{Events: raw}
		if !channelSubscribes(channel, EventDownloadComplete) {
			t.Fatalf("legacy events %q should still subscribe to all events", raw)
		}
	}
}

func TestChannelSubscribesSupportsExplicitAllAndSpecificEvents(t *testing.T) {
	all := model.NotifyChannel{Events: `["` + NotifyEventAll + `"]`}
	if !channelSubscribes(all, EventScrapeFailed) {
		t.Fatal("explicit all sentinel should subscribe to every event")
	}

	specific := model.NotifyChannel{Events: `["` + EventDownloadComplete + `"]`}
	if !channelSubscribes(specific, EventDownloadComplete) {
		t.Fatal("specific event should be subscribed")
	}
	if channelSubscribes(specific, EventScrapeFailed) {
		t.Fatal("unlisted event should not be subscribed")
	}
}
