package registry

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTruncateDigest(t *testing.T) {
	tests := []struct {
		name     string
		digest   string
		length   int
		expected string
	}{
		{
			name:     "digest shorter than length",
			digest:   "abc",
			length:   5,
			expected: "abc",
		},
		{
			name:     "digest equal to length",
			digest:   "abcde",
			length:   5,
			expected: "abcde",
		},
		{
			name:     "digest longer than length",
			digest:   "abcdefgh",
			length:   5,
			expected: "abcde",
		},
		{
			name:     "empty digest",
			digest:   "",
			length:   5,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TruncateDigest(tt.digest, tt.length)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasTag(t *testing.T) {
	tests := []struct {
		name     string
		tags     []string
		tag      string
		expected bool
	}{
		{
			name:     "tag exists",
			tags:     []string{"latest", "v1", "v2"},
			tag:      "v1",
			expected: true,
		},
		{
			name:     "tag does not exist",
			tags:     []string{"latest", "v1", "v2"},
			tag:      "v3",
			expected: false,
		},
		{
			name:     "empty tags",
			tags:     []string{},
			tag:      "latest",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasTag(tt.tags, tt.tag)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRemoveTagFromVersions(t *testing.T) {
	tests := []struct {
		name     string
		versions []VersionInfo
		tag      string
		expected []VersionInfo
	}{
		{
			name: "remove existing tag",
			versions: []VersionInfo{
				{Tags: []string{"latest", "v1"}},
				{Tags: []string{"v2", "v1"}},
			},
			tag: "v1",
			expected: []VersionInfo{
				{Tags: []string{"latest"}},
				{Tags: []string{"v2"}},
			},
		},
		{
			name: "remove non-existing tag",
			versions: []VersionInfo{
				{Tags: []string{"latest", "v1"}},
				{Tags: []string{"v2", "v1"}},
			},
			tag: "v3",
			expected: []VersionInfo{
				{Tags: []string{"latest", "v1"}},
				{Tags: []string{"v2", "v1"}},
			},
		},
		{
			name:     "empty versions",
			versions: []VersionInfo{},
			tag:      "latest",
			expected: []VersionInfo{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RemoveTagFromVersions(&tt.versions, tt.tag)
			assert.Equal(t, tt.expected, tt.versions)
		})
	}
}

func TestAddTagToVersion(t *testing.T) {
	tests := []struct {
		name         string
		versions     []VersionInfo
		shortDigest  string
		tag          string
		expectedTags []string
	}{
		{
			name: "add tag to existing version",
			versions: []VersionInfo{
				{Hash: "abc", Tags: []string{"latest"}},
				{Hash: "def", Tags: []string{"v1"}},
			},
			shortDigest:  "abc",
			tag:          "v2",
			expectedTags: []string{"latest", "v2"},
		},
		{
			name: "add tag to non-existing version",
			versions: []VersionInfo{
				{Hash: "abc", Tags: []string{"latest"}},
				{Hash: "def", Tags: []string{"v1"}},
			},
			shortDigest:  "ghi",
			tag:          "v2",
			expectedTags: []string{"latest"}, // No change expected
		},
		{
			name:         "empty versions",
			versions:     []VersionInfo{},
			shortDigest:  "abc",
			tag:          "v2",
			expectedTags: nil, // No change expected
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			AddTagToVersion(&tt.versions, tt.shortDigest, tt.tag)
			for _, version := range tt.versions {
				if version.Hash == tt.shortDigest {
					assert.Equal(t, tt.expectedTags, version.Tags)
					break
				}
			}
		})
	}
}

func TestRemoveTag(t *testing.T) {
	tests := []struct {
		name         string
		tags         []string
		tagToRemove  string
		expectedTags []string
	}{
		{
			name:         "remove existing tag",
			tags:         []string{"latest", "v1", "v2"},
			tagToRemove:  "v1",
			expectedTags: []string{"latest", "v2"},
		},
		{
			name:         "remove non-existing tag",
			tags:         []string{"latest", "v1", "v2"},
			tagToRemove:  "v3",
			expectedTags: []string{"latest", "v1", "v2"},
		},
		{
			name:         "empty tags",
			tags:         []string{},
			tagToRemove:  "latest",
			expectedTags: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RemoveTag(tt.tags, tt.tagToRemove)
			assert.Equal(t, tt.expectedTags, result)
		})
	}
}

func TestCreateVersionInfo(t *testing.T) {
	payload := []byte("test payload")

	tests := []struct {
		name         string
		shortDigest  string
		fullDigest   string
		payload      []byte
		tag          string
		expectedInfo VersionInfo
	}{
		{
			name:        "with tag",
			shortDigest: "abc123",
			fullDigest:  "abcdef123456",
			payload:     payload,
			tag:         "latest",
			expectedInfo: VersionInfo{
				Hash:       "abc123",
				FullDigest: "abcdef123456",
				Size:       int64(len(payload)),
				Tags:       []string{"latest"},
			},
		},
		{
			name:        "without tag",
			shortDigest: "abc123",
			fullDigest:  "abcdef123456",
			payload:     payload,
			tag:         "",
			expectedInfo: VersionInfo{
				Hash:       "abc123",
				FullDigest: "abcdef123456",
				Size:       int64(len(payload)),
				Tags:       []string{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Call the function
			result := CreateVersionInfo(tt.shortDigest, tt.fullDigest, tt.payload, tt.tag)

			// Ensure the CreatedAt field is set to a recent time
			assert.WithinDuration(t, time.Now(), result.CreatedAt, time.Second, "CreatedAt should be set to the current time")

			// Override the CreatedAt field in the expected result for comparison
			tt.expectedInfo.CreatedAt = result.CreatedAt
			assert.Equal(t, tt.expectedInfo, result)
		})
	}
}
