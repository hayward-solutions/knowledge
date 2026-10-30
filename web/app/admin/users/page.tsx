"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { Plus, Search } from "lucide-react";

import {
  createAdminUser,
  listAdminUsers,
  patchAdminUser,
  type PatchAdminUserInput,
  type User,
} from "@/lib/api";
import { PASSWORD_HINT, passwordPolicyOk } from "@/lib/password";
import { useMe } from "@/lib/auth";
import { Avatar } from "@/components/ui/Avatar";
import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { ConfirmDialog } from "@/components/ui/ConfirmDialog";
import { EmptyState } from "@/components/ui/EmptyState";
import { Field } from "@/components/ui/Field";
import { Input } from "@/components/ui/Input";
import { Modal } from "@/components/ui/Modal";
import { Spinner } from "@/components/ui/Spinner";
import { useToast } from "@/components/ui/Toast";

const USERS_KEY = ["admin-users"];

function ErrorBox({ text }: { text: string }) {
  return (
    <div
      role="alert"
      className="rounded-md border border-danger-200 bg-danger-50 px-3 py-2 text-sm text-danger-700"
    >
      {text}
    </div>
  );
}

function CreateUserModal({ onClose }: { onClose: () => void }) {
  const qc = useQueryClient();
  const toast = useToast();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [isAdmin, setIsAdmin] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const create = useMutation({
    mutationFn: () =>
      createAdminUser({
        email: email.trim().toLowerCase(),
        password,
        display_name: displayName.trim(),
        is_system_admin: isAdmin,
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: USERS_KEY });
      toast.success("User created");
      onClose();
    },
    onError: (e: Error) => setErr(e.message),
  });

  return (
    <Modal
      title="Create user"
      onClose={onClose}
      width={520}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button
            variant="primary"
            disabled={!email.trim() || !passwordPolicyOk(password) || create.isPending}
            onClick={() => create.mutate()}
          >
            {create.isPending ? "Creating…" : "Create user"}
          </Button>
        </>
      }
    >
      <div className="space-y-3">
        <Field label="Display name">
          <Input
            value={displayName}
            onChange={(e) => setDisplayName(e.target.value)}
            placeholder="Jane Doe"
          />
        </Field>
        <Field label="Email">
          <Input
            type="email"
            required
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="jane@company.com"
          />
        </Field>
        <Field label="Password" hint={PASSWORD_HINT}>
          <Input
            type="password"
            required
            autoComplete="new-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </Field>
        <label className="flex cursor-pointer items-center gap-2 text-sm text-ink-800">
          <input
            type="checkbox"
            checked={isAdmin}
            onChange={(e) => setIsAdmin(e.target.checked)}
            className="h-4 w-4 cursor-pointer accent-brand-600"
          />
          System administrator
        </label>
        {err && <ErrorBox text={err} />}
      </div>
    </Modal>
  );
}

function EditUserModal({
  user,
  onClose,
}: {
  user: User;
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const toast = useToast();
  const [displayName, setDisplayName] = useState(user.display_name);
  const [email, setEmail] = useState(user.email);
  const [err, setErr] = useState<string | null>(null);

  const save = useMutation({
    mutationFn: () =>
      patchAdminUser(user.id, {
        display_name: displayName.trim(),
        email: email.trim().toLowerCase(),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: USERS_KEY });
      toast.success("User updated");
      onClose();
    },
    onError: (e: Error) => setErr(e.message),
  });

  return (
    <Modal
      title="Edit user"
      onClose={onClose}
      width={480}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button
            variant="primary"
            disabled={!email.trim() || save.isPending}
            onClick={() => save.mutate()}
          >
            {save.isPending ? "Saving…" : "Save"}
          </Button>
        </>
      }
    >
      <div className="space-y-3">
        <Field label="Display name">
          <Input value={displayName} onChange={(e) => setDisplayName(e.target.value)} />
        </Field>
        <Field label="Email">
          <Input
            type="email"
            required
            value={email}
            onChange={(e) => setEmail(e.target.value)}
          />
        </Field>
        {err && <ErrorBox text={err} />}
      </div>
    </Modal>
  );
}

function ResetPasswordModal({
  user,
  onClose,
}: {
  user: User;
  onClose: () => void;
}) {
  const toast = useToast();
  const [pw, setPw] = useState("");
  const [err, setErr] = useState<string | null>(null);

  const reset = useMutation({
    mutationFn: () => patchAdminUser(user.id, { password: pw }),
    onSuccess: () => {
      toast.success("Password updated");
      onClose();
    },
    onError: (e: Error) => setErr(e.message),
  });

  return (
    <Modal
      title={`Reset password for ${user.display_name || user.email}`}
      onClose={onClose}
      width={480}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button
            variant="primary"
            disabled={!passwordPolicyOk(pw) || reset.isPending}
            onClick={() => reset.mutate()}
          >
            {reset.isPending ? "Saving…" : "Set password"}
          </Button>
        </>
      }
    >
      <div className="space-y-3">
        <Field label="New password" hint={PASSWORD_HINT}>
          <Input
            type="password"
            autoComplete="new-password"
            value={pw}
            onChange={(e) => setPw(e.target.value)}
          />
        </Field>
        {err && <ErrorBox text={err} />}
      </div>
    </Modal>
  );
}

export default function AdminUsersPage() {
  const qc = useQueryClient();
  const toast = useToast();
  const { data: me } = useMe();
  const users = useQuery({ queryKey: USERS_KEY, queryFn: listAdminUsers });

  const [search, setSearch] = useState("");
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<User | null>(null);
  const [resetting, setResetting] = useState<User | null>(null);
  const [disabling, setDisabling] = useState<User | null>(null);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    const list = users.data ?? [];
    if (!q) return list;
    return list.filter(
      (u) => u.email.toLowerCase().includes(q) || u.display_name.toLowerCase().includes(q)
    );
  }, [users.data, search]);

  const patch = useMutation({
    mutationFn: ({ id, input }: { id: string; input: PatchAdminUserInput }) =>
      patchAdminUser(id, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: USERS_KEY }),
    onError: (e: Error) => toast.error(e.message),
  });

  return (
    <>
      <div className="flex items-start justify-between gap-4 border-b border-ink-200 bg-ink-0 px-4 py-4 sm:px-6">
        <div>
          <h2 className="text-base font-semibold text-ink-900">Users</h2>
          <p className="text-sm text-ink-600">Create accounts, reset passwords, and grant admin access.</p>
        </div>
        <Button variant="primary" onClick={() => setCreating(true)}>
          <Plus size={14} /> New user
        </Button>
      </div>

      <div className="border-b border-ink-200 bg-ink-0 px-4 py-3 sm:px-6">
        <div className="relative max-w-sm">
          <Search
            size={14}
            className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-ink-400"
          />
          <Input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by name or email…"
            aria-label="Search users"
            className="pl-8"
          />
        </div>
      </div>

      <div className="flex-1 overflow-auto bg-ink-0">
        {users.isPending ? (
          <div className="flex justify-center py-16">
            <Spinner size={20} />
          </div>
        ) : users.isError ? (
          <div className="p-6">
            <ErrorBox text={users.error.message} />
          </div>
        ) : filtered.length === 0 ? (
          <div className="p-6">
            <EmptyState title={search ? "No users match your search" : "No users"} />
          </div>
        ) : (
          <table className="w-full min-w-[760px] border-separate border-spacing-0 text-sm">
            <thead className="sticky top-0 z-10 bg-ink-50 text-left text-[10.5px] font-semibold uppercase tracking-wider text-ink-600">
              <tr>
                <th className="border-b border-ink-200 px-4 py-2">User</th>
                <th className="border-b border-ink-200 px-3 py-2">Admin</th>
                <th className="border-b border-ink-200 px-3 py-2">Status</th>
                <th className="border-b border-ink-200 px-3 py-2">Created</th>
                <th className="border-b border-ink-200 px-4 py-2 text-right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((u) => {
                const label = u.display_name || u.email;
                const self = u.id === me?.id;
                return (
                  <tr key={u.id} className="hover:bg-brand-50/40">
                    <td className="border-b border-ink-100 px-4 py-2">
                      <div className="flex items-center gap-3">
                        <Avatar name={label} seed={u.id} size={28} />
                        <div className="min-w-0">
                          <div className="truncate font-medium text-ink-900">{label}</div>
                          <div className="truncate text-xs text-ink-500">{u.email}</div>
                        </div>
                      </div>
                    </td>
                    <td className="border-b border-ink-100 px-3 py-2">
                      <input
                        type="checkbox"
                        aria-label={`System administrator: ${label}`}
                        checked={u.is_system_admin}
                        disabled={self || patch.isPending}
                        onChange={(e) =>
                          patch.mutate({ id: u.id, input: { is_system_admin: e.target.checked } })
                        }
                        className="h-4 w-4 cursor-pointer accent-brand-600 disabled:cursor-not-allowed"
                      />
                    </td>
                    <td className="border-b border-ink-100 px-3 py-2">
                      {u.disabled_at ? (
                        <Badge tone="red">Disabled</Badge>
                      ) : (
                        <Badge tone="green">Active</Badge>
                      )}
                    </td>
                    <td className="whitespace-nowrap border-b border-ink-100 px-3 py-2 text-ink-600">
                      {new Date(u.created_at).toLocaleDateString()}
                    </td>
                    <td className="whitespace-nowrap border-b border-ink-100 px-4 py-2 text-right">
                      <Button size="sm" variant="ghost" onClick={() => setEditing(u)}>
                        Edit
                      </Button>
                      <Button size="sm" variant="ghost" className="ml-1" onClick={() => setResetting(u)}>
                        Reset password
                      </Button>
                      {u.disabled_at ? (
                        <Button
                          size="sm"
                          variant="secondary"
                          className="ml-1"
                          onClick={() => patch.mutate({ id: u.id, input: { disabled: false } })}
                        >
                          Enable
                        </Button>
                      ) : (
                        <Button
                          size="sm"
                          variant="ghost"
                          className="ml-1"
                          disabled={self}
                          onClick={() => setDisabling(u)}
                        >
                          Disable
                        </Button>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>

      {creating && <CreateUserModal onClose={() => setCreating(false)} />}
      {editing && <EditUserModal user={editing} onClose={() => setEditing(null)} />}
      {resetting && <ResetPasswordModal user={resetting} onClose={() => setResetting(null)} />}
      {disabling && (
        <ConfirmDialog
          title="Disable user"
          tone="danger"
          confirmLabel="Disable"
          busy={patch.isPending}
          message={`${disabling.display_name || disabling.email} will be signed out and unable to sign in until re-enabled.`}
          onCancel={() => setDisabling(null)}
          onConfirm={() =>
            patch.mutate(
              { id: disabling.id, input: { disabled: true } },
              { onSuccess: () => setDisabling(null) }
            )
          }
        />
      )}
    </>
  );
}
