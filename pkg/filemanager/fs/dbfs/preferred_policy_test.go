package dbfs

import (
	"testing"

	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
)

func fileWithPolicy(id int, policyID int, parent *File) *File {
	return &File{
		Model: &ent.File{
			ID:    id,
			Props: &types.FileProps{PreferredPolicyID: policyID},
		},
		Parent: parent,
	}
}

// TestPreferredPolicyIDInheritance covers the directory-level preference that
// decides where newly uploaded files are stored: the nearest ancestor folder
// with an explicit preference wins, and a missing preference means "use the
// group default" (0).
func TestPreferredPolicyIDInheritance(t *testing.T) {
	root := fileWithPolicy(1, 0, nil)
	parent := fileWithPolicy(2, 7, root)
	child := fileWithPolicy(3, 0, parent)
	grandChild := fileWithPolicy(4, 0, child)

	cases := []struct {
		name   string
		file   *File
		expect int
	}{
		{"explicit on itself", parent, 7},
		{"inherited from parent", child, 7},
		{"inherited from grand parent", grandChild, 7},
		{"nothing set in chain", root, 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.file.PreferredPolicyID(); got != c.expect {
				t.Fatalf("expected preferred policy %d, got %d", c.expect, got)
			}
		})
	}

	// A nearer preference must shadow a farther one.
	child.Model.Props.PreferredPolicyID = 9
	if got := grandChild.PreferredPolicyID(); got != 9 {
		t.Fatalf("expected nearest preference 9 to win, got %d", got)
	}

	// A nil props pointer anywhere in the chain must not panic.
	child.Model.Props = nil
	if got := grandChild.PreferredPolicyID(); got != 7 {
		t.Fatalf("expected to keep walking past nil props, got %d", got)
	}
}
