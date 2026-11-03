"use client";

import { useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { toast } from "sonner";
import { api } from "@multica/core/api";
import type { AgentRuntime, LLMModel } from "@multica/core/types";
import { useWorkspaceId } from "@multica/core/hooks";
import { useUpdateRuntime } from "@multica/core/runtimes/mutations";
import { Checkbox } from "@multica/ui/components/ui/checkbox";
import { useT } from "../../i18n";

/**
 * Per-runtime allowlist of enabled instance server LLM models. Empty means
 * agents on this runtime cannot pick (or inject) server catalog models.
 */
export function RuntimeServerModelsField({
  runtime,
  canEdit,
}: {
  runtime: AgentRuntime;
  canEdit: boolean;
}) {
  const { t } = useT("runtimes");
  const wsId = useWorkspaceId();
  const updateRuntime = useUpdateRuntime(wsId);
  const bound = useMemo(
    () => new Set(runtime.server_model_ids ?? []),
    [runtime.server_model_ids],
  );
  const [draft, setDraft] = useState<Set<string>>(() => new Set(bound));

  useEffect(() => {
    setDraft(new Set(bound));
  }, [bound]);

  const catalogQuery = useQuery({
    queryKey: ["llm", "models", "enabled"],
    queryFn: () => api.listEnabledLLMModels(),
    staleTime: 60_000,
  });
  const catalog = catalogQuery.data ?? [];

  const sameAsBound = (next: Set<string>) => {
    if (next.size !== bound.size) return false;
    for (const id of next) {
      if (!bound.has(id)) return false;
    }
    return true;
  };

  const commit = (next: Set<string>) => {
    setDraft(next);
    if (sameAsBound(next)) return;
    updateRuntime.mutate(
      {
        runtimeId: runtime.id,
        patch: { server_model_ids: [...next].sort() },
      },
      {
        onError: (err) => {
          setDraft(new Set(bound));
          toast.error(
            err instanceof Error && err.message
              ? err.message
              : t(($) => $.detail.toast_server_models_failed),
          );
        },
      },
    );
  };

  const toggle = (modelId: string, checked: boolean) => {
    const next = new Set(draft);
    if (checked) next.add(modelId);
    else next.delete(modelId);
    commit(next);
  };

  if (catalogQuery.isLoading) {
    return (
      <p className="text-caption text-muted-foreground">
        {t(($) => $.detail.server_models_loading)}
      </p>
    );
  }

  if (catalog.length === 0) {
    return (
      <p className="text-caption text-muted-foreground">
        {t(($) => $.detail.server_models_empty)}
      </p>
    );
  }

  if (!canEdit) {
    const labels = catalog
      .filter((m: LLMModel) => bound.has(m.model_id))
      .map((m) => m.label || m.model_id);
    return (
      <p className="text-caption text-muted-foreground">
        {labels.length > 0
          ? labels.join(" · ")
          : t(($) => $.detail.server_models_none)}
      </p>
    );
  }

  return (
    <ul className="space-y-2">
      {catalog.map((m) => {
        const checked = draft.has(m.model_id);
        return (
          <li key={m.model_id} className="flex items-start gap-2">
            <Checkbox
              checked={checked}
              onCheckedChange={(value) =>
                toggle(m.model_id, value === true)
              }
              disabled={updateRuntime.isPending}
              aria-label={m.label || m.model_id}
            />
            <span className="min-w-0">
              <span className="block truncate text-body">
                {m.label || m.model_id}
              </span>
              {m.label && m.label !== m.model_id ? (
                <span className="block truncate font-mono text-micro text-muted-foreground">
                  {m.model_id}
                </span>
              ) : null}
            </span>
          </li>
        );
      })}
    </ul>
  );
}
