import Link from "next/link";

export default function NotFound() {
  return (
    <main className="flex min-h-screen flex-col items-center justify-center gap-3 bg-ink-50 px-4 text-center">
      <div className="text-5xl font-semibold tracking-tight text-ink-300">404</div>
      <h1 className="text-lg font-semibold text-ink-900">Page not found</h1>
      <p className="max-w-sm text-sm text-ink-600">
        The page you are looking for does not exist, was moved, or you do not have access to it.
      </p>
      <Link href="/" className="btn btn-primary mt-2">
        Back to home
      </Link>
    </main>
  );
}
