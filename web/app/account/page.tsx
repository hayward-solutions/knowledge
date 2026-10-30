"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";

import { AppShell } from "@/components/layout/AppShell";
import { Breadcrumbs } from "@/components/layout/Breadcrumbs";
import { changePassword, updateMe } from "@/lib/api";
import { ME_QUERY_KEY, useMe } from "@/lib/auth";
import { PASSWORD_HINT, passwordPolicyOk } from "@/lib/password";
import { Avatar } from "@/components/ui/Avatar";
import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { Field } from "@/components/ui/Field";
import { Input } from "@/components/ui/Input";
import { Spinner } from "@/components/ui/Spinner";
import { useToast } from "@/components/ui/Toast";

function Message({ tone, text }: { tone: "ok" | "err"; text: string }) {
  return (
    <div
      role={tone === "err" ? "alert" : "status"}
      className={`rounded-md border px-3 py-2 text-sm ${
        tone === "ok"
          ? "border-success-100 bg-success-50 text-success-700"
          : "border-danger-200 bg-danger-50 text-danger-700"
      }`}
    >
      {text}
    </div>
  );
}

export default function AccountPage() {
  const qc = useQueryClient();
  const toast = useToast();
  const { data: me } = useMe();

  const [displayName, setDisplayName] = useState("");
  const [email, setEmail] = useState("");
  const [profileErr, setProfileErr] = useState<string | null>(null);

  useEffect(() => {
    if (me) {
      setDisplayName(me.display_name);
      setEmail(me.email);
    }
  }, [me]);

  const saveProfile = useMutation({
    mutationFn: () =>
      updateMe({ display_name: displayName.trim(), email: email.trim() }),
    onSuccess: (user) => {
      qc.setQueryData(ME_QUERY_KEY, user);
      setProfileErr(null);
      toast.success("Profile saved");
    },
    onError: (e: Error) => setProfileErr(e.message),
  });

  const [currentPw, setCurrentPw] = useState("");
  const [newPw, setNewPw] = useState("");
  const [newPw2, setNewPw2] = useState("");
  const [pwErr, setPwErr] = useState<string | null>(null);

  const changePw = useMutation({
    mutationFn: () => changePassword(currentPw, newPw),
    onSuccess: () => {
      setCurrentPw("");
      setNewPw("");
      setNewPw2("");
      setPwErr(null);
      toast.success("Password changed");
    },
    onError: (e: Error) => setPwErr(e.message),
  });

  const label = me?.display_name || me?.email || "Account";

  return (
    <AppShell topSlot={<Breadcrumbs items={[{ label: "Account" }]} />}>
      <div className="border-b border-ink-200 bg-ink-0 px-4 py-4 sm:px-6">
        <h1 className="text-[20px] font-semibold tracking-tight text-ink-900">Account</h1>
        <p className="text-sm text-ink-600">Manage your profile and password.</p>
      </div>

      <div className="flex-1 overflow-auto p-4 sm:p-6">
        {!me ? (
          <div className="flex justify-center py-12">
            <Spinner size={20} />
          </div>
        ) : (
          <div className="mx-auto max-w-2xl space-y-6">
            <section className="surface flex items-center gap-4 p-5">
              <Avatar name={label} seed={me.id} size={56} />
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <span className="truncate text-base font-semibold text-ink-900">{label}</span>
                  {me.is_system_admin && (
                    <Badge tone="purple" bold>
                      Server admin
                    </Badge>
                  )}
                </div>
                <div className="truncate text-sm text-ink-600">{me.email}</div>
                <div className="mt-1 text-xs text-ink-500">
                  Joined {new Date(me.created_at).toLocaleDateString()}
                </div>
              </div>
            </section>

            <section className="surface p-5">
              <div className="mb-4">
                <h2 className="text-sm font-semibold text-ink-900">Profile</h2>
                <p className="text-xs text-ink-500">
                  Your display name is shown on pages, comments, and activity.
                </p>
              </div>
              <form
                className="space-y-3"
                onSubmit={(e) => {
                  e.preventDefault();
                  saveProfile.mutate();
                }}
              >
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
                {profileErr && <Message tone="err" text={profileErr} />}
                <div>
                  <Button type="submit" variant="primary" disabled={saveProfile.isPending}>
                    {saveProfile.isPending ? "Saving…" : "Save changes"}
                  </Button>
                </div>
              </form>
            </section>

            <section className="surface p-5">
              <div className="mb-4">
                <h2 className="text-sm font-semibold text-ink-900">Change password</h2>
                <p className="text-xs text-ink-500">{PASSWORD_HINT}</p>
              </div>
              <form
                className="space-y-3"
                onSubmit={(e) => {
                  e.preventDefault();
                  if (!passwordPolicyOk(newPw)) {
                    setPwErr(PASSWORD_HINT);
                    return;
                  }
                  if (newPw !== newPw2) {
                    setPwErr("New passwords do not match.");
                    return;
                  }
                  setPwErr(null);
                  changePw.mutate();
                }}
              >
                <Field label="Current password">
                  <Input
                    type="password"
                    required
                    autoComplete="current-password"
                    value={currentPw}
                    onChange={(e) => setCurrentPw(e.target.value)}
                  />
                </Field>
                <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                  <Field label="New password">
                    <Input
                      type="password"
                      required
                      autoComplete="new-password"
                      value={newPw}
                      onChange={(e) => setNewPw(e.target.value)}
                    />
                  </Field>
                  <Field label="Confirm new password">
                    <Input
                      type="password"
                      required
                      autoComplete="new-password"
                      value={newPw2}
                      onChange={(e) => setNewPw2(e.target.value)}
                    />
                  </Field>
                </div>
                {pwErr && <Message tone="err" text={pwErr} />}
                <div>
                  <Button type="submit" variant="primary" disabled={changePw.isPending}>
                    {changePw.isPending ? "Changing…" : "Change password"}
                  </Button>
                </div>
              </form>
            </section>
          </div>
        )}
      </div>
    </AppShell>
  );
}
