package domain

import "testing"

func TestRoleAllowsMatrix(t *testing.T) {
	cases := []struct {
		perm   Perm
		viewer bool
		editor bool
		admin  bool
	}{
		{PermViewSpace, true, true, true},
		{PermComment, true, true, true},
		{PermEditPages, false, true, true},
		{PermManageAttachments, false, true, true},
		{PermManageLabels, false, true, true},
		{PermManageTemplates, false, true, true},
		{PermManageMembers, false, false, true},
		{PermManageSpace, false, false, true},
		{PermPurgeTrash, false, false, true},
		{PermDeleteSpace, false, false, true},
	}
	for _, c := range cases {
		if got := RoleAllows(RoleViewer, c.perm); got != c.viewer {
			t.Errorf("viewer %s = %v, want %v", c.perm, got, c.viewer)
		}
		if got := RoleAllows(RoleEditor, c.perm); got != c.editor {
			t.Errorf("editor %s = %v, want %v", c.perm, got, c.editor)
		}
		if got := RoleAllows(RoleAdmin, c.perm); got != c.admin {
			t.Errorf("admin %s = %v, want %v", c.perm, got, c.admin)
		}
	}
}

func TestRoleAllowsUnknown(t *testing.T) {
	if RoleAllows(Role("nobody"), PermViewSpace) {
		t.Error("unknown role must not be allowed")
	}
	if RoleAllows(RoleAdmin, Perm("unknown")) {
		t.Error("unknown perm must not be allowed")
	}
	if RoleAllows("", PermViewSpace) {
		t.Error("empty role must not be allowed")
	}
}

func TestRoleAtLeast(t *testing.T) {
	if !RoleAdmin.AtLeast(RoleEditor) || !RoleEditor.AtLeast(RoleViewer) || RoleViewer.AtLeast(RoleEditor) {
		t.Error("role ordering broken")
	}
	if !RoleAdmin.Valid() || Role("x").Valid() {
		t.Error("role validity broken")
	}
}
