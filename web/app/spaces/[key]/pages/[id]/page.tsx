"use client";

import { useParams } from "next/navigation";
import { PageBody } from "@/components/pages/PageBody";

export default function PageRedirectRoute() {
  const params = useParams<{ id: string }>();
  const id = String(params.id);
  return <PageBody key={id} pageId={id} />;
}
