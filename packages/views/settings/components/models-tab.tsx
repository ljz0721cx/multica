"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Loader2, Plus, RefreshCw, Trash2 } from "@multica/ui/icons";
import { toast } from "sonner";
import type { DiscoverLLMModel, LLMModel } from "@multica/core/types";
import { api } from "@multica/core/api";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import { Checkbox } from "@multica/ui/components/ui/checkbox";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@multica/ui/components/ui/alert-dialog";
import { useT } from "../../i18n";
import {
  SettingsCard,
  SettingsSection,
  SettingsTab,
} from "./settings-layout";

export function ModelsTab() {
  const { t } = useT("settings");
  const [baseUrl, setBaseUrl] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [hasApiKey, setHasApiKey] = useState(false);
  const [configLoading, setConfigLoading] = useState(true);
  const [configSaving, setConfigSaving] = useState(false);
  const [models, setModels] = useState<LLMModel[]>([]);
  const [modelsLoading, setModelsLoading] = useState(true);
  const [addOpen, setAddOpen] = useState(false);
  const [modelId, setModelId] = useState("");
  const [modelLabel, setModelLabel] = useState("");
  const [creating, setCreating] = useState(false);
  const [busyId, setBusyId] = useState<string | null>(null);
  const [deleting, setDeleting] = useState<LLMModel | null>(null);

  const [discoverOpen, setDiscoverOpen] = useState(false);
  const [discovering, setDiscovering] = useState(false);
  const [discovered, setDiscovered] = useState<DiscoverLLMModel[]>([]);
  const [discoverFilter, setDiscoverFilter] = useState("");
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [syncing, setSyncing] = useState(false);

  const load = useCallback(async () => {
    setConfigLoading(true);
    setModelsLoading(true);
    try {
      const [config, list] = await Promise.all([
        api.getLLMConfigAdmin(),
        api.listLLMModelsAdmin(),
      ]);
      setBaseUrl(config.base_url ?? "");
      setHasApiKey(!!config.has_api_key);
      setApiKey("");
      setModels(list);
    } catch (e) {
      toast.error(
        e instanceof Error ? e.message : t(($) => $.models.toast_load_failed),
      );
    } finally {
      setConfigLoading(false);
      setModelsLoading(false);
    }
  }, [t]);

  useEffect(() => {
    void load();
  }, [load]);

  const handleSaveConfig = async () => {
    setConfigSaving(true);
    try {
      const payload: { base_url: string; api_key?: string } = {
        base_url: baseUrl.trim(),
      };
      if (apiKey.trim()) {
        payload.api_key = apiKey.trim();
      }
      const saved = await api.putLLMConfigAdmin(payload);
      setBaseUrl(saved.base_url ?? "");
      setHasApiKey(!!saved.has_api_key);
      setApiKey("");
      toast.success(t(($) => $.models.toast_config_saved));
    } catch (e) {
      toast.error(
        e instanceof Error ? e.message : t(($) => $.models.toast_config_failed),
      );
    } finally {
      setConfigSaving(false);
    }
  };

  const openDiscover = async () => {
    setDiscoverOpen(true);
    setDiscovering(true);
    setDiscoverFilter("");
    try {
      const list = await api.discoverLLMModelsAdmin();
      setDiscovered(list);
      // Default: already-enabled catalog models + newly discovered ones.
      setSelectedIds(
        new Set(
          list
            .filter((m) => (m.in_catalog && m.enabled) || !m.in_catalog)
            .map((m) => m.model_id),
        ),
      );
    } catch (e) {
      setDiscovered([]);
      setSelectedIds(new Set());
      toast.error(
        e instanceof Error
          ? e.message
          : t(($) => $.models.toast_discover_failed),
      );
    } finally {
      setDiscovering(false);
    }
  };

  const filteredDiscovered = useMemo(() => {
    const needle = discoverFilter.trim().toLowerCase();
    if (!needle) return discovered;
    return discovered.filter(
      (m) =>
        m.model_id.toLowerCase().includes(needle) ||
        m.label.toLowerCase().includes(needle),
    );
  }, [discovered, discoverFilter]);

  const toggleSelected = (id: string, next: boolean) => {
    setSelectedIds((prev) => {
      const copy = new Set(prev);
      if (next) copy.add(id);
      else copy.delete(id);
      return copy;
    });
  };

  const handleSync = async () => {
    if (selectedIds.size === 0) return;
    setSyncing(true);
    try {
      await api.syncLLMModelsAdmin({ model_ids: [...selectedIds] });
      toast.success(
        t(($) => $.models.toast_sync_saved, { count: selectedIds.size }),
      );
      setDiscoverOpen(false);
      await load();
    } catch (e) {
      toast.error(
        e instanceof Error ? e.message : t(($) => $.models.toast_sync_failed),
      );
    } finally {
      setSyncing(false);
    }
  };

  const handleCreate = async () => {
    if (!modelId.trim()) return;
    setCreating(true);
    try {
      await api.createLLMModelAdmin({
        model_id: modelId.trim(),
        label: modelLabel.trim() || undefined,
        enabled: true,
      });
      setAddOpen(false);
      setModelId("");
      setModelLabel("");
      toast.success(t(($) => $.models.toast_model_created));
      await load();
    } catch (e) {
      toast.error(
        e instanceof Error
          ? e.message
          : t(($) => $.models.toast_model_create_failed),
      );
    } finally {
      setCreating(false);
    }
  };

  const handleToggle = async (model: LLMModel) => {
    setBusyId(model.id);
    try {
      await api.updateLLMModelAdmin(model.id, { enabled: !model.enabled });
      toast.success(t(($) => $.models.toast_model_updated));
      await load();
    } catch (e) {
      toast.error(
        e instanceof Error
          ? e.message
          : t(($) => $.models.toast_model_update_failed),
      );
    } finally {
      setBusyId(null);
    }
  };

  const handleDelete = async () => {
    if (!deleting) return;
    setBusyId(deleting.id);
    try {
      await api.deleteLLMModelAdmin(deleting.id);
      setDeleting(null);
      toast.success(t(($) => $.models.toast_model_deleted));
      await load();
    } catch (e) {
      toast.error(
        e instanceof Error
          ? e.message
          : t(($) => $.models.toast_model_delete_failed),
      );
    } finally {
      setBusyId(null);
    }
  };

  const canDiscover = !configLoading && (!!baseUrl.trim() || hasApiKey);

  return (
    <SettingsTab title={t(($) => $.page.tabs.models)} scope="account">
      <SettingsSection
        title={t(($) => $.models.gateway_title)}
        anchor="gateway"
      >
        {configLoading ? (
          <div className="flex items-center gap-2 text-body text-muted-foreground">
            <Loader2 className="size-4 animate-spin" />
          </div>
        ) : (
          <div className="space-y-4 rounded-xl border border-border p-4">
            <div className="space-y-1.5">
              <Label htmlFor="llm-base-url">{t(($) => $.models.base_url_label)}</Label>
              <Input
                id="llm-base-url"
                value={baseUrl}
                onChange={(e) => setBaseUrl(e.target.value)}
                placeholder={t(($) => $.models.base_url_placeholder)}
                spellCheck={false}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="llm-api-key">{t(($) => $.models.api_key_label)}</Label>
              <Input
                id="llm-api-key"
                type="password"
                autoComplete="off"
                value={apiKey}
                onChange={(e) => setApiKey(e.target.value)}
                placeholder={t(($) => $.models.api_key_placeholder)}
              />
              {hasApiKey ? (
                <p className="text-caption text-muted-foreground">
                  {t(($) => $.models.api_key_set)}
                </p>
              ) : null}
            </div>
            <div className="flex flex-wrap gap-2">
              <Button
                onClick={() => void handleSaveConfig()}
                disabled={configSaving}
                aria-busy={configSaving || undefined}
              >
                {configSaving
                  ? t(($) => $.models.saving_config)
                  : t(($) => $.models.save_config)}
              </Button>
              <Button
                variant="outline"
                onClick={() => void openDiscover()}
                disabled={!canDiscover || discovering}
              >
                <RefreshCw />
                {t(($) => $.models.discover_models)}
              </Button>
            </div>
          </div>
        )}
      </SettingsSection>

      <SettingsSection
        title={t(($) => $.models.catalog_title)}
        anchor="catalog"
        action={
          <div className="flex items-center gap-2">
            <Button
              size="sm"
              variant="outline"
              onClick={() => void openDiscover()}
              disabled={!canDiscover || discovering}
            >
              <RefreshCw />
              {t(($) => $.models.discover_models)}
            </Button>
            <Button size="sm" onClick={() => setAddOpen(true)}>
              <Plus />
              {t(($) => $.models.add_model)}
            </Button>
          </div>
        }
      >
        {modelsLoading ? (
          <div className="flex items-center gap-2 text-body text-muted-foreground">
            <Loader2 className="size-4 animate-spin" />
          </div>
        ) : models.length === 0 ? (
          <p className="text-body text-muted-foreground">{t(($) => $.models.empty)}</p>
        ) : (
          <SettingsCard>
            {models.map((model) => (
              <div
                key={model.id}
                className="flex items-center gap-3 px-4 py-3"
              >
                <div className="min-w-0 flex-1">
                  <div className="truncate text-body font-medium">
                    {model.label || model.model_id}
                  </div>
                  {model.label && model.label !== model.model_id ? (
                    <div className="truncate text-caption text-muted-foreground">
                      {model.model_id}
                    </div>
                  ) : null}
                </div>
                <Badge variant={model.enabled ? "secondary" : "outline"}>
                  {model.enabled
                    ? t(($) => $.models.enabled_label)
                    : t(($) => $.models.disable)}
                </Badge>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={busyId === model.id}
                  onClick={() => void handleToggle(model)}
                >
                  {model.enabled
                    ? t(($) => $.models.disable)
                    : t(($) => $.models.enable)}
                </Button>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  disabled={busyId === model.id}
                  onClick={() => setDeleting(model)}
                  aria-label={t(($) => $.models.delete)}
                >
                  <Trash2 className="size-4 text-muted-foreground" />
                </Button>
              </div>
            ))}
          </SettingsCard>
        )}
      </SettingsSection>

      <Dialog
        open={discoverOpen}
        onOpenChange={(open) => {
          if (!open && !syncing) setDiscoverOpen(false);
        }}
      >
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t(($) => $.models.discover_title)}</DialogTitle>
          </DialogHeader>
          {discovering ? (
            <div className="flex items-center gap-2 py-8 text-body text-muted-foreground">
              <Loader2 className="size-4 animate-spin" />
              {t(($) => $.models.discovering)}
            </div>
          ) : (
            <div className="space-y-3">
              <Input
                value={discoverFilter}
                onChange={(e) => setDiscoverFilter(e.target.value)}
                placeholder={t(($) => $.models.discover_filter_placeholder)}
                spellCheck={false}
              />
              {filteredDiscovered.length === 0 ? (
                <p className="text-body text-muted-foreground">
                  {t(($) => $.models.discover_empty)}
                </p>
              ) : (
                <div className="max-h-72 space-y-1 overflow-y-auto rounded-lg border border-border p-2">
                  {filteredDiscovered.map((model) => {
                    const checked = selectedIds.has(model.model_id);
                    return (
                      <label
                        key={model.model_id}
                        className="flex cursor-pointer items-center gap-3 rounded-md px-2 py-2 hover:bg-muted/50"
                      >
                        <Checkbox
                          checked={checked}
                          onCheckedChange={(value) =>
                            toggleSelected(model.model_id, value === true)
                          }
                        />
                        <span className="min-w-0 flex-1 truncate text-body">
                          {model.label || model.model_id}
                        </span>
                        {model.in_catalog && model.enabled ? (
                          <Badge variant="secondary">
                            {t(($) => $.models.enabled_label)}
                          </Badge>
                        ) : null}
                      </label>
                    );
                  })}
                </div>
              )}
            </div>
          )}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => setDiscoverOpen(false)}
              disabled={syncing}
            >
              {t(($) => $.models.cancel)}
            </Button>
            <Button
              type="button"
              disabled={syncing || discovering || selectedIds.size === 0}
              aria-busy={syncing || undefined}
              onClick={() => void handleSync()}
            >
              {syncing
                ? t(($) => $.models.syncing)
                : t(($) => $.models.sync_selected, {
                    count: selectedIds.size,
                  })}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={addOpen}
        onOpenChange={(open) => {
          if (!open && !creating) setAddOpen(false);
        }}
      >
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t(($) => $.models.add_model)}</DialogTitle>
          </DialogHeader>
          <form
            className="space-y-4"
            onSubmit={(event) => {
              event.preventDefault();
              if (modelId.trim()) void handleCreate();
            }}
          >
            <div className="space-y-1.5">
              <Label htmlFor="new-model-id">{t(($) => $.models.model_id_label)}</Label>
              <Input
                id="new-model-id"
                value={modelId}
                onChange={(e) => setModelId(e.target.value)}
                placeholder={t(($) => $.models.model_id_placeholder)}
                spellCheck={false}
                autoFocus
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="new-model-label">{t(($) => $.models.label_label)}</Label>
              <Input
                id="new-model-label"
                value={modelLabel}
                onChange={(e) => setModelLabel(e.target.value)}
                placeholder={t(($) => $.models.label_placeholder)}
              />
            </div>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setAddOpen(false)}
                disabled={creating}
              >
                {t(($) => $.models.cancel)}
              </Button>
              <Button
                type="submit"
                disabled={creating || !modelId.trim()}
                aria-busy={creating || undefined}
              >
                {creating
                  ? t(($) => $.models.creating)
                  : t(($) => $.models.create_button)}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <AlertDialog
        open={!!deleting}
        onOpenChange={(open) => {
          if (!open) setDeleting(null);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t(($) => $.models.delete_confirm_title)}</AlertDialogTitle>
            <AlertDialogDescription>
              {t(($) => $.models.delete_confirm_description, {
                label: deleting?.label || deleting?.model_id || "",
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t(($) => $.models.cancel)}</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              onClick={() => void handleDelete()}
            >
              {t(($) => $.models.delete)}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </SettingsTab>
  );
}
