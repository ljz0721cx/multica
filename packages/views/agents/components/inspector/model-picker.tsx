"use client";

import { useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronDown, Cpu, Loader2, Plus } from "@multica/ui/icons";
import { api } from "@multica/core/api";
import {
  refreshRuntimeModels,
  runtimeModelsOptions,
} from "@multica/core/runtimes";
import type { RuntimeModel } from "@multica/core/types";
import { Label } from "@multica/ui/components/ui/label";
import {
  PickerItem,
  PropertyPicker,
} from "../../../issues/components/pickers";
import { CHIP_CLASS } from "./chip";
import { useT } from "../../../i18n";
import { UnavailableModelsNote } from "../unavailable-models-note";
import { ModelSearchHeader } from "../model-search-header";
import { TooltipHint } from "@multica/ui/components/common/tooltip-hint";

const SERVER_PROVIDER = "Server";

/**
 * Inline model picker for the agent inspector. Lighter cousin of
 * `ModelDropdown` (which is used in the create-agent dialog) — same data
 * sources (runtime catalog + enabled server models), but renders inside a
 * PropertyPicker so it fits a single PropRow. Drops the "select a runtime
 * first" state because the inspector only renders this picker after a
 * runtime is bound.
 *
 * Providers whose runtime ignores per-agent model selection report
 * `supported=false` and render an inert italic "Managed by runtime" label
 * when no server catalog models are available. Server models still remain
 * selectable so claim-time gateway injection can apply.
 */
export function ModelPicker({
  runtimeId,
  runtimeOnline,
  value,
  canEdit = true,
  variant = "chip",
  showLabel = true,
  onChange,
}: {
  runtimeId: string | null;
  runtimeOnline: boolean;
  value: string;
  /** When false, render a static read-only display and skip the popover. */
  canEdit?: boolean;
  variant?: "chip" | "field";
  showLabel?: boolean;
  onChange: (next: string) => Promise<void> | void;
}) {
  const { t } = useT("agents");
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState("");

  const modelsQuery = useQuery(
    runtimeModelsOptions(runtimeOnline ? runtimeId : null),
  );
  const serverModelsQuery = useQuery({
    queryKey: ["llm", "models", "enabled"],
    queryFn: () => api.listEnabledLLMModels(),
    staleTime: 60_000,
  });
  const supported = modelsQuery.data?.supported ?? true;
  // Memoise the model list so every downstream useMemo gets a stable
  // reference; `?? []` would mint a fresh array on every render and
  // invalidate filters needlessly.
  // Advisory only — never merged into `models`, so no row below can select one.
  const unavailableModels = useMemo(
    () => modelsQuery.data?.unavailableModels ?? [],
    [modelsQuery.data],
  );
  const serverModels = useMemo<RuntimeModel[]>(
    () =>
      (serverModelsQuery.data ?? []).map((m) => ({
        id: m.model_id,
        label: m.label || m.model_id,
        provider: SERVER_PROVIDER,
      })),
    [serverModelsQuery.data],
  );
  const models = useMemo(
    () => [...serverModels, ...(modelsQuery.data?.models ?? [])],
    [serverModels, modelsQuery.data],
  );
  const grouped = useMemo(() => {
    const map = new Map<string, RuntimeModel[]>();
    for (const m of models) {
      const key = m.provider || "";
      const list = map.get(key);
      if (list) list.push(m);
      else map.set(key, [m]);
    }
    return map;
  }, [models]);

  const filtered = useMemo(() => {
    const s = search.trim().toLowerCase();
    if (!s) return grouped;
    const out = new Map<string, RuntimeModel[]>();
    for (const [provider, list] of grouped) {
      const hit = list.filter(
        (m) =>
          m.id.toLowerCase().includes(s) || m.label.toLowerCase().includes(s),
      );
      if (hit.length > 0) out.set(provider, hit);
    }
    return out;
  }, [grouped, search]);

  const trimmedSearch = search.trim();
  const exactMatch = models.some(
    (m) => m.id === trimmedSearch || m.label === trimmedSearch,
  );
  const canCreate = trimmedSearch.length > 0 && !exactMatch;
  const filteredCount = useMemo(() => {
    let n = 0;
    for (const list of filtered.values()) n += list.length;
    return n;
  }, [filtered]);
  const listsLoading = modelsQuery.isLoading && serverModelsQuery.isLoading;

  const triggerLabel = value || t(($) => $.pickers.model_default);
  const triggerTitle = t(($) => $.pickers.model_tooltip, { value: triggerLabel });

  const select = async (id: string) => {
    setOpen(false);
    setSearch("");
    if (id !== value) await onChange(id);
  };

  const refresh = () => {
    if (!runtimeId || !runtimeOnline) return;
    void refreshRuntimeModels(queryClient, runtimeId).catch(() => {
      // React Query retains the last catalog and owns the error state. Avoid
      // turning a failed button action into an unhandled promise rejection.
    });
  };

  if (!supported && !modelsQuery.isLoading && serverModels.length === 0) {
    if (variant === "field") {
      const control = (
        <div className="flex min-h-10 items-center gap-2 rounded-lg border border-dashed border-input bg-input/50 px-3 text-body text-muted-foreground">
          <Cpu className="h-4 w-4 shrink-0" aria-hidden="true" />
          <span className="truncate italic">
            {t(($) => $.pickers.model_managed_by_runtime)}
          </span>
        </div>
      );
      if (!showLabel) return control;
      return (
        <div className="flex min-w-0 flex-col">
          <Label>{t(($) => $.inspector.prop_model)}</Label>
          <div className="mt-1.5">{control}</div>
        </div>
      );
    }
    return (
      <span className="truncate italic text-muted-foreground">
        {t(($) => $.pickers.model_managed_by_runtime)}
      </span>
    );
  }

  if (!canEdit) {
    if (variant === "field") {
      const control = (
        <div className="flex min-h-10 items-center gap-2 rounded-lg border border-input bg-input/50 px-3 text-body text-muted-foreground">
          <Cpu className="h-4 w-4 shrink-0" aria-hidden="true" />
          <span className="min-w-0 truncate font-mono">{triggerLabel}</span>
        </div>
      );
      if (!showLabel) return control;
      return (
        <div className="flex min-w-0 flex-col">
          <Label>{t(($) => $.inspector.prop_model)}</Label>
          <div className="mt-1.5">{control}</div>
        </div>
      );
    }
    return (
      <span
        className="min-w-0 truncate px-1.5 py-0.5 font-mono text-micro text-muted-foreground"
        title={triggerTitle}
      >
        {triggerLabel}
      </span>
    );
  }

  const picker = (
    <PropertyPicker
      open={open}
      onOpenChange={setOpen}
      width={
        variant === "field"
          ? "w-[var(--anchor-width)] min-w-[16rem] max-w-md"
          : "w-auto min-w-[16rem] max-w-md"
      }
      align="start"
      tooltip={triggerTitle}
      triggerRender={
        <button
          type="button"
          className={
            variant === "field"
              ? `${showLabel ? "mt-1.5 " : ""}flex min-h-10 w-full min-w-0 items-center gap-2 rounded-lg border border-input bg-transparent px-3 text-left text-body transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50`
              : CHIP_CLASS
          }
          aria-label={triggerTitle}
        />
      }
      trigger={
        <>
          {variant === "field" ? (
            <Cpu
              className="h-4 w-4 shrink-0 text-muted-foreground"
              aria-hidden="true"
            />
          ) : null}
          <span
            className={
              variant === "field"
                ? "min-w-0 flex-1 truncate font-mono"
                : "min-w-0 truncate font-mono text-micro"
            }
          >
            {triggerLabel}
          </span>
          {variant === "field" ? (
            <ChevronDown
              className={`h-4 w-4 shrink-0 text-muted-foreground transition-transform ${
                open ? "rotate-180" : ""
              }`}
              aria-hidden="true"
            />
          ) : null}
        </>
      }
      header={
        <ModelSearchHeader
          value={search}
          onChange={setSearch}
          onRefresh={refresh}
          refreshing={modelsQuery.isFetching}
          refreshDisabled={!runtimeOnline || !runtimeId}
          compact
        />
      }
    >
      {listsLoading && (
        <div className="flex items-center gap-2 p-3 text-caption text-muted-foreground">
          <Loader2
            className="h-3 w-3 animate-spin motion-reduce:animate-none"
            aria-hidden="true"
          />
          {t(($) => $.pickers.model_discovering)}
        </div>
      )}

      {!listsLoading &&
        [...filtered.entries()].map(([provider, list]) => (
          <div key={provider || "runtime"}>
            {provider ? (
              <div className="px-3 pt-2 pb-1 text-micro font-medium uppercase tracking-wide text-muted-foreground">
                {provider === SERVER_PROVIDER
                  ? t(($) => $.model_dropdown.server_provider)
                  : provider}
              </div>
            ) : null}
            {list.map((m) => (
              <PickerItem
                key={`${provider}:${m.id}`}
                selected={m.id === value}
                onClick={() => void select(m.id)}
                // Tooltip carries the canonical model id even when the chip
                // shows the friendlier label, so users can always see what
                // string actually ships to the agent.
                tooltip={m.label !== m.id ? `${m.label} · ${m.id}` : m.id}
              >
                {/* PickerItem wraps children in a flex `<span>`. Putting a
                    `<div>` inside that <span> is block-in-inline (invalid
                    HTML5) and triggers the browser-default centering quirk
                    that pushes descendants off-axis (model IDs floated to the
                    center instead of left-aligning under their labels). Use
                    `<span block text-left>` to keep layout deterministic —
                    matches the fix already applied in thinking-picker.tsx. */}
                <span className="block min-w-0 flex-1 text-left">
                  <span className="block truncate text-label font-medium">
                    {m.label}
                  </span>
                  {m.label !== m.id && (
                    <span className="mt-0.5 block truncate font-mono text-micro leading-snug text-muted-foreground">
                      {m.id}
                    </span>
                  )}
                </span>
              </PickerItem>
            ))}
          </div>
        ))}

      {!listsLoading && filteredCount === 0 && !canCreate && (
        <p className="px-3 py-3 text-center text-caption text-muted-foreground">
          {t(($) => $.pickers.model_empty)}
        </p>
      )}

      {!listsLoading && (
        <UnavailableModelsNote
          models={unavailableModels}
          title={t(($) => $.pickers.model_unavailable_heading)}
        />
      )}

      {canCreate && (
        <PickerItem
          selected={false}
          onClick={() => void select(trimmedSearch)}
          tooltip={t(($) => $.pickers.model_custom_tooltip, { value: trimmedSearch })}
        >
          <Plus className="h-3.5 w-3.5 shrink-0 text-primary" />
          <span className="truncate text-primary">
            {t(($) => $.pickers.model_custom_use, { value: trimmedSearch })}
          </span>
        </PickerItem>
      )}

      {value && (
        <TooltipHint label={t(($) => $.pickers.model_clear_title)}>
          <button
            type="button"
            onClick={() => void select("")}
            className="mt-1 flex w-full items-center border-t px-3 py-2 text-left text-caption text-muted-foreground transition-colors hover:bg-accent/50"
          >
            {t(($) => $.pickers.model_clear)}
          </button>
        </TooltipHint>
      )}
    </PropertyPicker>
  );

  if (variant === "field") {
    if (!showLabel) return picker;
    return (
      <div className="flex min-w-0 flex-col">
        <Label>{t(($) => $.inspector.prop_model)}</Label>
        {picker}
      </div>
    );
  }

  return picker;
}
