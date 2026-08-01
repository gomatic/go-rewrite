package rewrite

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildPlan(t *testing.T) {
	t.Parallel()
	want := assert.New(t)

	plan := BuildPlan(sourceIdentity(), targetIdentityValue())

	want.Equal(FilePath("cmd/before.cli"), plan.MoveFrom)
	want.Equal(FilePath("cmd/after.cli"), plan.MoveTo)
	want.Equal([]Replacement{
		{From: "example.com/org/before.cli", To: "example.com/org/after.cli"},
		{From: "BEFORE_CLI", To: "AFTER_CLI"},
		{From: "beforecli", To: "aftercli"},
		{From: "before.cli", To: "after.cli"},
	}, plan.Replacements)
}

func TestApplySkipsEmptyAndIdenticalReplacements(t *testing.T) {
	t.Parallel()
	want, must := assert.New(t), require.New(t)

	fs := &fakeFS{
		files:   []FilePath{"f"},
		data:    map[FilePath][]byte{"f": []byte("keep abc xyz"), goModFile: nil},
		readErr: map[FilePath]error{},
	}
	plan := Plan{
		Replacements: []Replacement{
			{From: "", To: "ignored"},  // empty From is skipped
			{From: "abc", To: "abc"},   // identical is skipped
			{From: "xyz", To: "final"}, // applied
		},
	}

	changed, err := plan.Apply(fs, false)

	must.NoError(err)
	want.Equal(Changed{"f"}, changed)
	want.Equal("keep abc final", string(fs.data["f"]))
}
