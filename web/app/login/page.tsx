"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { Suspense, useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { BookOpen } from "lucide-react";

import { login, oidcConfig, oidcLoginUrl, type OIDCConfig } from "@/lib/api";
import { ME_QUERY_KEY, safeNext } from "@/lib/auth";
import { Button } from "@/components/ui/Button";
import { Field } from "@/components/ui/Field";
import { Input } from "@/components/ui/Input";
import { Spinner } from "@/components/ui/Spinner";

function LoginForm() {
  const router = useRouter();
  const qc = useQueryClient();
  const params = useSearchParams();
  const next = safeNext(params?.get("next"));
  const ssoError = params?.get("error");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [err, setErr] = useState<string | null>(ssoError);
  const [busy, setBusy] = useState(false);
  const [oidc, setOidc] = useState<OIDCConfig | null>(null);

  useEffect(() => {
    oidcConfig()
      .then(setOidc)
      .catch(() => setOidc({ enabled: false }));
  }, []);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErr(null);
    try {
      const user = await login(email.trim(), password);
      qc.setQueryData(ME_QUERY_KEY, user);
      router.push(next);
    } catch (e) {
      setErr(e instanceof Error ? e.message : "Login failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit} className="space-y-3">
      <Field label="Email">
        <Input
          type="email"
          required
          autoComplete="username"
          placeholder="you@company.com"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          autoFocus
        />
      </Field>
      <Field label="Password">
        <Input
          type="password"
          required
          autoComplete="current-password"
          placeholder="••••••••"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />
      </Field>
      {err && (
        <div
          role="alert"
          className="rounded-md border border-danger-200 bg-danger-50 px-3 py-2 text-sm text-danger-700"
        >
          {err}
        </div>
      )}
      <Button type="submit" variant="primary" size="lg" disabled={busy} className="w-full">
        {busy ? "Signing in…" : "Sign in"}
      </Button>
      {oidc?.enabled && (
        <>
          <div className="relative my-2 flex items-center">
            <div className="flex-grow border-t border-ink-200" />
            <span className="mx-3 text-xs uppercase tracking-wide text-ink-500">or</span>
            <div className="flex-grow border-t border-ink-200" />
          </div>
          <a
            href={oidcLoginUrl(next)}
            className="flex h-11 w-full items-center justify-center rounded-md border border-ink-200 bg-ink-0 px-4 text-sm font-medium text-ink-900 shadow-sm hover:bg-ink-100"
          >
            Sign in with {oidc.provider_name || "SSO"}
          </a>
        </>
      )}
    </form>
  );
}

export default function LoginPage() {
  return (
    <main className="relative flex min-h-screen items-center justify-center overflow-hidden bg-gradient-to-br from-brand-50 via-ink-0 to-purple-50 px-4 py-12">
      <div
        aria-hidden
        className="pointer-events-none absolute -left-20 top-1/4 h-72 w-72 rounded-full bg-brand-200/50 blur-3xl"
      />
      <div
        aria-hidden
        className="pointer-events-none absolute -right-20 bottom-1/4 h-72 w-72 rounded-full bg-purple-100/60 blur-3xl"
      />

      <div className="relative w-full max-w-[400px]">
        <div className="mb-6 flex items-center justify-center gap-2">
          <span className="flex h-10 w-10 items-center justify-center rounded-lg bg-gradient-to-br from-brand-500 to-brand-700 text-white shadow-card">
            <BookOpen size={22} />
          </span>
          <span className="text-xl font-semibold tracking-tight text-ink-900">Knowledge</span>
        </div>

        <div className="surface p-6 shadow-card">
          <div className="mb-5 text-center">
            <h1 className="text-lg font-semibold text-ink-900">Sign in to continue</h1>
            <p className="mt-1 text-sm text-ink-600">
              Write, organise, and share what your team knows.
            </p>
          </div>

          <Suspense
            fallback={
              <div className="flex justify-center py-8">
                <Spinner size={20} />
              </div>
            }
          >
            <LoginForm />
          </Suspense>
        </div>

        <p className="mt-4 text-center text-xs text-ink-500">
          Need an account? Ask a server admin to invite you.
        </p>
      </div>
    </main>
  );
}
