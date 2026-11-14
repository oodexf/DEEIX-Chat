"use client";

import { Pencil, Plus, Trash2 } from "lucide-react";
import { useTranslations } from "next-intl";
import * as React from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
import { useShellProjectMemories } from "@/features/shell/hooks/use-shell-project-memories";
import type { ProjectMemoryDTO } from "@/shared/api/memory-types";

// Memories shared by all conversations of a project: written by the user here,
// by the assistant through its memory tools, or extracted after each reply.
export function ProjectMemoryPanel({ projectID }: { projectID: string }) {
  const t = useTranslations("recent.projects.memory");
  const { items, loading, saveMemory, deleteMemory } = useShellProjectMemories(projectID);
  const [newKey, setNewKey] = React.useState("");
  const [newValue, setNewValue] = React.useState("");
  const [adding, setAdding] = React.useState(false);

  const handleAdd = async () => {
    const key = newKey.trim();
    const value = newValue.trim();
    if (!key || !value || adding) return;
    setAdding(true);
    if (await saveMemory(key, value)) {
      setNewKey("");
      setNewValue("");
    }
    setAdding(false);
  };

  return (
    <div className="space-y-3">
      <p className="text-xs text-muted-foreground">{t("description")}</p>
      <div className="flex items-start gap-2">
        <Input
          value={newKey}
          maxLength={128}
          placeholder={t("keyPlaceholder")}
          className="h-8 w-40 shrink-0 text-xs"
          disabled={adding}
          onChange={(event) => setNewKey(event.target.value)}
        />
        <Input
          value={newValue}
          maxLength={2000}
          placeholder={t("valuePlaceholder")}
          className="h-8 min-w-0 flex-1 text-xs"
          disabled={adding}
          onChange={(event) => setNewValue(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter") void handleAdd();
          }}
        />
        <Button
          type="button"
          size="sm"
          className="h-8 gap-1 px-2"
          disabled={adding || !newKey.trim() || !newValue.trim()}
          onClick={() => void handleAdd()}
        >
          <Plus className="size-3.5" strokeWidth={1.6} />
          {t("add")}
        </Button>
      </div>

      {loading ? (
        <div className="space-y-1">
          <Skeleton className="h-9 w-full rounded-md" />
          <Skeleton className="h-9 w-4/5 rounded-md" />
        </div>
      ) : items.length === 0 ? (
        <div className="flex h-9 items-center rounded-md bg-muted/30 px-2.5">
          <p className="text-xs text-muted-foreground">{t("empty")}</p>
        </div>
      ) : (
        <div className="space-y-1">
          {items.map((item) => (
            <ProjectMemoryRow key={item.id} item={item} onSave={saveMemory} onDelete={deleteMemory} />
          ))}
        </div>
      )}
    </div>
  );
}

function ProjectMemoryRow({
  item,
  onSave,
  onDelete,
}: {
  item: ProjectMemoryDTO;
  onSave: (memoryKey: string, value: string) => Promise<boolean>;
  onDelete: (memoryID: number) => Promise<void>;
}) {
  const t = useTranslations("recent.projects.memory");
  const [editingValue, setEditingValue] = React.useState<string | null>(null);
  const [saving, setSaving] = React.useState(false);

  const commitEdit = async () => {
    const value = editingValue?.trim() ?? "";
    if (!value || saving) return;
    setSaving(true);
    if (await onSave(item.memoryKey, value)) setEditingValue(null);
    setSaving(false);
  };

  if (editingValue !== null) {
    return (
      <div className="rounded-md border border-border/60 px-2.5 py-2">
        <p className="mb-1 text-[11px] font-medium text-foreground/80">{item.memoryKey}</p>
        <Textarea
          autoFocus
          rows={3}
          value={editingValue}
          maxLength={2000}
          className="min-h-16 resize-none text-xs"
          disabled={saving}
          onChange={(event) => setEditingValue(event.target.value)}
        />
        <div className="mt-2 flex justify-end gap-1">
          <Button type="button" size="sm" variant="ghost" className="h-6 px-2 text-[11px]" disabled={saving} onClick={() => setEditingValue(null)}>
            {t("cancel")}
          </Button>
          <Button type="button" size="sm" className="h-6 px-2 text-[11px]" disabled={saving || !editingValue.trim()} onClick={() => void commitEdit()}>
            {t("save")}
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className="group flex min-h-9 items-start gap-2 rounded-md px-2 py-1.5 transition-colors hover:bg-muted/40">
      <div className="min-w-0 flex-1">
        <p className="flex items-center gap-1.5 text-xs leading-5">
          <span className="truncate font-medium text-foreground/80">{item.memoryKey}</span>
          <Badge variant="secondary" className="h-4 shrink-0 rounded-md px-1.5 text-[9px] font-normal">
            {t(`source.${item.source}`)}
          </Badge>
        </p>
        <p className="whitespace-pre-wrap break-words text-xs leading-5 text-muted-foreground">{item.value}</p>
      </div>
      <div className="flex shrink-0 gap-0.5 opacity-100 transition-opacity md:opacity-0 md:group-hover:opacity-100 md:group-focus-within:opacity-100">
        <button
          type="button"
          className="inline-flex size-6 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
          onClick={() => setEditingValue(item.value)}
          aria-label={t("edit")}
        >
          <Pencil className="size-3" />
        </button>
        <button
          type="button"
          className="inline-flex size-6 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-destructive"
          onClick={() => void onDelete(item.id)}
          aria-label={t("delete")}
        >
          <Trash2 className="size-3" />
        </button>
      </div>
    </div>
  );
}
