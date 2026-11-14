"use client";

import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeightTransition,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { SpinnerLabel } from "@/components/ui/spinner";
import { Textarea } from "@/components/ui/textarea";
import type { KnowledgeBaseDocumentDraft } from "@/features/knowledge-bases/types/knowledge-bases";
import { useDialogSnapshot } from "@/shared/hooks/use-dialog-snapshot";

// Writes a Markdown text document straight into a personal knowledge base.
export function KnowledgeBaseDocumentDialog({
  draft,
  saving,
  onDraftChange,
  onClose,
  onSave,
}: {
  draft: KnowledgeBaseDocumentDraft | null;
  saving: boolean;
  onDraftChange: (draft: KnowledgeBaseDocumentDraft) => void;
  onClose: () => void;
  onSave: () => void;
}) {
  const t = useTranslations("knowledgeBases");
  const stableDraft = useDialogSnapshot(draft);

  return (
    <Dialog open={Boolean(draft)} onOpenChange={(open) => !open && !saving && onClose()}>
      <DialogContent className="w-[calc(100vw-2rem)] gap-0 overflow-hidden p-0 sm:max-w-[720px]">
        <DialogHeightTransition contentClassName="max-h-[min(86vh,760px)]">
          <DialogHeader className="shrink-0 px-4 py-4">
            <DialogTitle>{stableDraft?.fileID ? t("editDocument") : t("newDocument")}</DialogTitle>
            <DialogDescription>{t("documentDescription")}</DialogDescription>
          </DialogHeader>
          <form
            className="flex min-h-0 flex-1 flex-col"
            onSubmit={(event) => {
              event.preventDefault();
              onSave();
            }}
          >
            <div className="min-h-0 flex-1 space-y-4 overflow-y-auto px-4 py-2">
              <div className="space-y-1">
                <label className="text-xs font-normal text-muted-foreground" htmlFor="knowledge-base-document-title">
                  {t("documentTitle")}
                </label>
                <Input
                  id="knowledge-base-document-title"
                  autoFocus
                  maxLength={255}
                  value={stableDraft?.title ?? ""}
                  placeholder={t("documentTitlePlaceholder")}
                  disabled={saving}
                  required
                  onChange={(event) => draft && onDraftChange({ ...draft, title: event.target.value })}
                />
              </div>
              <div className="space-y-1">
                <label className="text-xs font-normal text-muted-foreground" htmlFor="knowledge-base-document-content">
                  {t("documentContent")}
                </label>
                <Textarea
                  id="knowledge-base-document-content"
                  className="h-80 resize-none overflow-y-auto font-mono text-xs [field-sizing:fixed]"
                  value={stableDraft?.content ?? ""}
                  placeholder={t("documentContentPlaceholder")}
                  disabled={saving}
                  required
                  onChange={(event) => draft && onDraftChange({ ...draft, content: event.target.value })}
                />
              </div>
            </div>
            <DialogFooter className="shrink-0 px-4 py-3">
              <Button type="button" variant="ghost" disabled={saving} onClick={onClose}>{t("cancel")}</Button>
              <Button type="submit" disabled={!stableDraft?.title.trim() || !stableDraft.content.trim() || saving}>
                {saving ? <SpinnerLabel>{t("save")}</SpinnerLabel> : t("save")}
              </Button>
            </DialogFooter>
          </form>
        </DialogHeightTransition>
      </DialogContent>
    </Dialog>
  );
}
