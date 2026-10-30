"use client";

import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Trash2, UserPlus, X } from "lucide-react";
import {
  ApiError,
  listSpaceMembers,
  putSpaceMember,
  removeSpaceMember,
  searchUsers,
  type Role,
  type SpaceMember,
  type UserRef,
} from "@/lib/api";
import { useMe } from "@/lib/auth";
import { Avatar } from "@/components/ui/Avatar";
import { Button } from "@/components/ui/Button";
import { ConfirmDialog } from "@/components/ui/ConfirmDialog";
import { EmptyState } from "@/components/ui/EmptyState";
import { Field } from "@/components/ui/Field";
import { Input } from "@/components/ui/Input";
import { Select } from "@/components/ui/Select";
import { useToast } from "@/components/ui/Toast";
import { ListSkeleton } from "@/components/pages/Skeletons";
import { errorMessage } from "@/components/pages/utils";
import { SettingsShell } from "@/components/spaces/SettingsShell";
import { useSpace } from "@/components/spaces/SpaceContext";

const ROLES: { value: Role; label: string }[] = [
  { value: "admin", label: "Admin" },
  { value: "editor", label: "Editor" },
  { value: "viewer", label: "Viewer" },
];

function useDebounced<T>(value: T, ms: number): T {
  const [v, setV] = useState(value);
  useEffect(() => {
    const t = window.setTimeout(() => setV(value), ms);
    return () => window.clearTimeout(t);
  }, [value, ms]);
  return v;
}

function memberError(err: unknown, fallback: string): string {
  if (err instanceof ApiError && err.code === "last_admin") {
    return "A space must keep at least one admin.";
  }
  return errorMessage(err, fallback);
}

function AddMember({ spaceKey, existing }: { spaceKey: string; existing: Set<string> }) {
  const qc = useQueryClient();
  const toast = useToast();
  const [q, setQ] = useState("");
  const [picked, setPicked] = useState<UserRef | null>(null);
  const [role, setRole] = useState<Role>("viewer");
  const debounced = useDebounced(q.trim(), 250);

  const results = useQuery<UserRef[], Error>({
    queryKey: ["user-search", debounced],
    queryFn: () => searchUsers(debounced),
    enabled: debounced.length >= 1 && !picked,
    staleTime: 15_000,
  });
  const candidates = (results.data ?? []).filter((u) => !existing.has(u.id));

  const add = useMutation({
    mutationFn: () => putSpaceMember(spaceKey, (picked as UserRef).id, role),
    onSuccess: async () => {
      toast.success(`Added ${picked?.display_name}`);
      setPicked(null);
      setQ("");
      setRole("viewer");
      await qc.invalidateQueries({ queryKey: ["members", spaceKey] });
      void qc.invalidateQueries({ queryKey: ["spaces"] });
    },
    onError: (err) => toast.error(memberError(err, "Could not add member")),
  });

  return (
    <form
      className="surface p-4"
      onSubmit={(e) => {
        e.preventDefault();
        if (picked) add.mutate();
      }}
    >
      <h2 className="mb-3 text-sm font-semibold text-ink-900">Add a member</h2>
      <div className="flex flex-wrap items-end gap-3">
        <div className="relative min-w-[220px] flex-1">
          <Field label="Find a user">
            {picked ? (
              <div className="control flex w-full items-center gap-2">
                <Avatar name={picked.display_name} seed={picked.id} size={20} />
                <span className="min-w-0 flex-1 truncate">{picked.display_name}</span>
                <button
                  type="button"
                  aria-label="Clear selected user"
                  onClick={() => setPicked(null)}
                  className="text-ink-500 hover:text-ink-900"
                >
                  <X size={14} />
                </button>
              </div>
            ) : (
              <Input
                value={q}
                onChange={(e) => setQ(e.target.value)}
                placeholder="Search by name or email"
                autoComplete="off"
              />
            )}
          </Field>
          {!picked && debounced.length >= 1 && (
            <ul
              role="listbox"
              aria-label="Matching users"
              className="absolute left-0 right-0 z-20 mt-1 max-h-56 overflow-y-auto rounded-md border border-ink-200 bg-ink-0 py-1 shadow-overlay"
            >
              {results.isPending && <li className="px-3 py-2 text-sm text-ink-600">Searching...</li>}
              {!results.isPending && candidates.length === 0 && (
                <li className="px-3 py-2 text-sm text-ink-600">No matching users.</li>
              )}
              {candidates.map((u) => (
                <li key={u.id} role="option" aria-selected={false}>
                  <button
                    type="button"
                    onClick={() => setPicked(u)}
                    className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm hover:bg-ink-100 focus:bg-ink-100 focus:outline-none"
                  >
                    <Avatar name={u.display_name} seed={u.id} size={22} />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-ink-900">{u.display_name}</span>
                      {u.email && <span className="block truncate text-xs text-ink-600">{u.email}</span>}
                    </span>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
        <Field label="Role">
          <Select value={role} onChange={(e) => setRole(e.target.value as Role)}>
            {ROLES.map((r) => (
              <option key={r.value} value={r.value}>
                {r.label}
              </option>
            ))}
          </Select>
        </Field>
        <Button variant="primary" type="submit" disabled={!picked || add.isPending}>
          <UserPlus size={14} />
          {add.isPending ? "Adding..." : "Add"}
        </Button>
      </div>
    </form>
  );
}

export default function MembersSettingsPage() {
  const qc = useQueryClient();
  const toast = useToast();
  const { data: me } = useMe();
  const { space, isAdmin } = useSpace();
  const [toRemove, setToRemove] = useState<SpaceMember | null>(null);
  const key = space.key;

  const query = useQuery<SpaceMember[], Error>({
    queryKey: ["members", key],
    queryFn: () => listSpaceMembers(key),
    enabled: isAdmin,
    retry: false,
  });

  const changeRole = useMutation({
    mutationFn: ({ member, role }: { member: SpaceMember; role: Role }) =>
      putSpaceMember(key, member.user_id, role),
    onSuccess: async (_m, { member, role }) => {
      toast.success(`${member.user.display_name} is now ${role === "admin" ? "an" : "a"} ${role}`);
      await qc.invalidateQueries({ queryKey: ["members", key] });
    },
    onError: async (err) => {
      toast.error(memberError(err, "Could not change role"));
      await qc.invalidateQueries({ queryKey: ["members", key] });
    },
  });

  const remove = useMutation({
    mutationFn: (m: SpaceMember) => removeSpaceMember(key, m.user_id),
    onSuccess: async (_v, m) => {
      toast.success(`Removed ${m.user.display_name}`);
      setToRemove(null);
      await qc.invalidateQueries({ queryKey: ["members", key] });
      void qc.invalidateQueries({ queryKey: ["spaces"] });
    },
    onError: (err) => {
      setToRemove(null);
      toast.error(memberError(err, "Could not remove member"));
    },
  });

  const members = [...(query.data ?? [])].sort((a, b) => a.user.display_name.localeCompare(b.user.display_name));
  const existing = new Set(members.map((m) => m.user_id));

  return (
    <SettingsShell active="members">
      <div className="space-y-6">
        <AddMember spaceKey={key} existing={existing} />

        <section aria-label="Members">
          {query.isPending ? (
            <ListSkeleton rows={4} />
          ) : query.isError ? (
            <EmptyState
              title="Could not load members"
              description={query.error.message}
              action={
                <Button variant="primary" onClick={() => void query.refetch()}>
                  Try again
                </Button>
              }
            />
          ) : members.length === 0 ? (
            <EmptyState
              title="No members yet"
              description={
                space.visibility === "internal"
                  ? "Everyone signed in can read this space. Add members to give them edit access."
                  : "Add members to give them access to this private space."
              }
            />
          ) : (
            <ul className="surface divide-y divide-ink-100">
              {members.map((m) => (
                <li key={m.user_id} className="flex flex-wrap items-center gap-3 px-4 py-3">
                  <Avatar name={m.user.display_name} seed={m.user_id} size={32} />
                  <div className="min-w-0 flex-1 basis-48">
                    <div className="truncate text-sm font-medium text-ink-900">
                      {m.user.display_name}
                      {m.user_id === me?.id && <span className="ml-1.5 text-xs font-normal text-ink-500">(you)</span>}
                    </div>
                    {m.user.email && <div className="truncate text-xs text-ink-600">{m.user.email}</div>}
                  </div>
                  <Select
                    aria-label={`Role for ${m.user.display_name}`}
                    value={m.role}
                    disabled={changeRole.isPending && changeRole.variables?.member.user_id === m.user_id}
                    onChange={(e) => changeRole.mutate({ member: m, role: e.target.value as Role })}
                  >
                    {ROLES.map((r) => (
                      <option key={r.value} value={r.value}>
                        {r.label}
                      </option>
                    ))}
                  </Select>
                  <Button
                    variant="ghost"
                    size="sm"
                    aria-label={`Remove ${m.user.display_name}`}
                    onClick={() => setToRemove(m)}
                  >
                    <Trash2 size={14} />
                    Remove
                  </Button>
                </li>
              ))}
            </ul>
          )}
        </section>
      </div>

      {toRemove && (
        <ConfirmDialog
          title="Remove member"
          tone="danger"
          confirmLabel="Remove"
          busy={remove.isPending}
          onConfirm={() => remove.mutate(toRemove)}
          onCancel={() => setToRemove(null)}
          message={
            <>
              <strong>{toRemove.user.display_name}</strong> will lose their {toRemove.role} access to this space
              {space.visibility === "internal" ? ", but can still read it because the space is internal" : ""}.
            </>
          }
        />
      )}
    </SettingsShell>
  );
}
