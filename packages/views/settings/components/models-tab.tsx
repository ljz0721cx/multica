"use client";

import { useCallback, useEffect, useState } from "react";
import { Loader2, Plus, Trash2 } from "@multica/ui/icons";
import { toast } from "sonner";
import type { LLMModel } from "@multica/core/types";
import { api } from "@multica/core/api";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
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
            <div>
              <Button
                onClick={() => void handleSaveConfig()}
                disabled={configSaving}
                aria-busy={configSaving || undefined}
              >
                {configSaving
                  ? t(($) => $.models.saving_config)
                  : t(($) => $.models.save_config)}
              </Button>
            </div>
          </div>
        )}
      </SettingsSection>

      <SettingsSection
        title={t(($) => $.models.catalog_title)}
        anchor="catalog"
        action={
          <Button size="sm" onClick={() => setAddOpen(true)}>
            <Plus />
            {t(($) => $.models.add_model)}
          </Button>
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
