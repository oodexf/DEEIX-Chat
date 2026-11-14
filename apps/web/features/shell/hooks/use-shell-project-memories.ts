import { useTranslations } from "next-intl";
import * as React from "react";
import { toast } from "sonner";

import { useLocalizedErrorMessage } from "@/i18n/use-localized-error";
import { deleteProjectMemory, listProjectMemories, upsertProjectMemory } from "@/shared/api/memory";
import type { ProjectMemoryDTO } from "@/shared/api/memory-types";
import { resolveAccessToken } from "@/shared/auth/resolve-access-token";

/**
 * Memories shared by every conversation in one project. Saving reuses the
 * upsert endpoint, so adding an existing key replaces that row in place.
 */
export function useShellProjectMemories(projectID: string) {
  const t = useTranslations("recent.projects.memory");
  const resolveErrorMessage = useLocalizedErrorMessage();
  const [items, setItems] = React.useState<ProjectMemoryDTO[]>([]);
  const [loading, setLoading] = React.useState(true);

  React.useEffect(() => {
    let active = true;
    setLoading(true);
    void (async () => {
      try {
        const token = await resolveAccessToken();
        if (!token) return;
        const memories = await listProjectMemories(token, projectID);
        if (active) setItems(memories);
      } catch (error) {
        if (active) toast.error(t("loadFailed"), { description: resolveErrorMessage(error) });
      } finally {
        if (active) setLoading(false);
      }
    })();
    return () => {
      active = false;
    };
  }, [projectID, resolveErrorMessage, t]);

  const saveMemory = React.useCallback(async (memoryKey: string, value: string): Promise<boolean> => {
    try {
      const token = await resolveAccessToken();
      if (!token) return false;
      const memory = await upsertProjectMemory(token, projectID, memoryKey, value);
      setItems((current) => [memory, ...current.filter((item) => item.id !== memory.id)]);
      return true;
    } catch (error) {
      toast.error(t("saveFailed"), { description: resolveErrorMessage(error) });
      return false;
    }
  }, [projectID, resolveErrorMessage, t]);

  const deleteMemory = React.useCallback(async (memoryID: number) => {
    try {
      const token = await resolveAccessToken();
      if (!token) return;
      await deleteProjectMemory(token, projectID, memoryID);
      setItems((current) => current.filter((item) => item.id !== memoryID));
    } catch (error) {
      toast.error(t("deleteFailed"), { description: resolveErrorMessage(error) });
    }
  }, [projectID, resolveErrorMessage, t]);

  return { items, loading, saveMemory, deleteMemory };
}
