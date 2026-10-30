package domain

type Perm string

const (
	PermViewSpace         Perm = "view_space"
	PermComment           Perm = "comment"
	PermEditPages         Perm = "edit_pages"
	PermManageAttachments Perm = "manage_attachments"
	PermManageLabels      Perm = "manage_labels"
	PermManageTemplates   Perm = "manage_templates"
	PermManageMembers     Perm = "manage_members"
	PermManageSpace       Perm = "manage_space"
	PermPurgeTrash        Perm = "purge_trash"
	PermDeleteSpace       Perm = "delete_space"
)

func RoleAllows(role Role, perm Perm) bool {
	switch perm {
	case PermViewSpace, PermComment:
		return role.AtLeast(RoleViewer)
	case PermEditPages, PermManageAttachments, PermManageLabels, PermManageTemplates:
		return role.AtLeast(RoleEditor)
	case PermManageMembers, PermManageSpace, PermPurgeTrash, PermDeleteSpace:
		return role.AtLeast(RoleAdmin)
	}
	return false
}
