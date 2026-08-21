package vngcloud_repo

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
)

func tagRepo() *vngCloudRepository {
	return &vngCloudRepository{
		tagCache: newTTLCache[[]entityv2.Tag](time.Hour, 16),
	}
}

// ListTags must answer from the cache without touching the SDK. The repository here has a
// nil client on purpose: if the read-through path were not taken, the call would panic
// instead of returning.
func TestListTagsServesFromCacheWithoutCallingSDK(t *testing.T) {
	repo := tagRepo()
	repo.tagCache.put("lb-123", []entityv2.Tag{
		{Key: "vng.vks.cluster.ids", Value: "k8s-1/k8s-2"},
		{Key: "vng.billing.product", Value: "vks"},
	})

	got, err := repo.ListTags(context.Background(), "lb-123")

	require.NoError(t, err)
	require.Len(t, got.Items, 2)
	assert.Equal(t, "k8s-1/k8s-2", got.Items[0].Value)
}

// Every hit builds its own copy, so one caller cannot corrupt what the next one reads.
func TestListTagsHitsDoNotShareMutableTags(t *testing.T) {
	repo := tagRepo()
	repo.tagCache.put("lb-123", []entityv2.Tag{{Key: "team", Value: "vks"}})

	first, err := repo.ListTags(context.Background(), "lb-123")
	require.NoError(t, err)
	first.Items[0].Value = "scribbled"

	second, err := repo.ListTags(context.Background(), "lb-123")
	require.NoError(t, err)
	assert.Equal(t, "vks", second.Items[0].Value, "a hit must not hand out the cached Tag itself")
}

func TestInvalidateTagsCacheForcesAReadThrough(t *testing.T) {
	repo := tagRepo()
	repo.tagCache.put("lb-123", []entityv2.Tag{{Key: "team", Value: "vks"}})

	repo.InvalidateTagsCache("lb-123")

	// Nothing is remembered any more, so the next ListTags has to go to the SDK - which is
	// nil here, hence the panic. That panic is the evidence.
	assert.Panics(t, func() {
		_, _ = repo.ListTags(context.Background(), "lb-123") //nolint:errcheck
	})
}

// A write changes the resource's tags, so the cached copy must go - and it must go even
// when the write fails, since a failed call can still have applied part of the change.
func TestTagWritesForgetTheCachedTagsEvenOnFailure(t *testing.T) {
	for _, tc := range []struct {
		name  string
		write func(*vngCloudRepository) error
	}{
		{"CreateTags", func(r *vngCloudRepository) error {
			return r.CreateTags(context.Background(), "lb-123", map[string]string{"team": "vks"})
		}},
		{"UpdateTags", func(r *vngCloudRepository) error {
			return r.UpdateTags(context.Background(), "lb-123", map[string]string{"team": "vks"})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := tagRepo()
			repo.tagCache.put("lb-123", []entityv2.Tag{{Key: "team", Value: "stale"}})

			// The nil client makes the SDK call panic, which stands in for the write failing.
			func() {
				defer func() { _ = recover() }()
				_ = tc.write(repo) //nolint:errcheck
			}()

			_, ok := repo.tagCache.get("lb-123")
			assert.False(t, ok, "a write must not leave a stale cached copy behind")
		})
	}
}

func TestTagValuesSkipsNils(t *testing.T) {
	assert.Nil(t, tagValues(nil))
	assert.Equal(t,
		[]entityv2.Tag{{Key: "a", Value: "1"}},
		tagValues(&entityv2.ListTags{Items: []*entityv2.Tag{nil, {Key: "a", Value: "1"}}}))
}
