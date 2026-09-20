import { Box, Button, Dialog, DialogActions, DialogContent, DialogTitle } from "@mui/material";
import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { getAvailablePolicies, getFileInfo, sendRelocate } from "../../../api/api.ts";
import { AvailableStoragePolicy } from "../../../api/explorer.ts";
import { useAppDispatch, useAppSelector } from "../../../redux/hooks.ts";
import { closeRelocateDialog } from "../../../redux/globalStateSlice.ts";
import { DenseSelect } from "../../Common/StyledComponents";
import { SquareMenuItem } from "../ContextMenu/ContextMenu.tsx";

/**
 * Moves the data of the selected files and folders to another storage policy.
 *
 * The picker opens on the policy the selection currently lives on, matching how the
 * folder-level policy dialogs behave: the field shows the current state rather than an
 * empty box. That policy is also listed, so the value shown is always selectable.
 */
const RelocateDialog = () => {
  const { t } = useTranslation();
  const dispatch = useAppDispatch();
  const open = useAppSelector((s) => s.globalState.relocateDialogOpen);
  const files = useAppSelector((s) => s.globalState.relocateDialogFiles);

  const [policies, setPolicies] = useState<AvailableStoragePolicy[]>([]);
  const [currentPolicyID, setCurrentPolicyID] = useState<string>("");
  const [target, setTarget] = useState<string>("");
  const [loading, setLoading] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  // Where the selection currently lives. `extended_info` is only filled in by the
  // single-file detail endpoint, not by a listing, so it is fetched when the dialog
  // opens: without it the picker could not open on the current policy.
  useEffect(() => {
    if (!open) {
      return;
    }

    const list = files ?? [];
    if (list.length === 0) {
      setCurrentPolicyID("");
      return;
    }

    let cancelled = false;
    Promise.all(
      list.map((f) =>
        f.path
          ? dispatch(getFileInfo({ uri: f.path, extended: true }))
              .then((res) => res.extended_info?.storage_policy?.id ?? "")
              .catch(() => "")
          : Promise.resolve(""),
      ),
    ).then((ids) => {
      if (cancelled) {
        return;
      }

      // The most common policy describes the selection; ties keep the first seen.
      const counts = new Map<string, number>();
      ids.filter(Boolean).forEach((id) => counts.set(id, (counts.get(id) ?? 0) + 1));

      let best = "";
      let bestCount = 0;
      for (const [id, count] of counts) {
        if (count > bestCount) {
          best = id;
          bestCount = count;
        }
      }

      setCurrentPolicyID(best);
    });

    return () => {
      cancelled = true;
    };
  }, [open, files, dispatch]);

  useEffect(() => {
    if (!open) {
      return;
    }

    setLoading(true);
    dispatch(getAvailablePolicies({}))
      .then((res) => {
        setPolicies(res.policies ?? []);
      })
      .finally(() => setLoading(false));
  }, [open, dispatch]);

  // Preselect the policy in use, so the field reflects the current state on open.
  useEffect(() => {
    if (open) {
      setTarget(currentPolicyID);
    }
  }, [open, currentPolicyID]);

  const onClose = useCallback(() => {
    dispatch(closeRelocateDialog());
  }, [dispatch]);

  // Relocating onto the policy the selection already lives on moves nothing, so such a
  // task would be created only to report zero files moved.
  const isSamePolicy = target !== "" && target === currentPolicyID;

  const onSubmit = useCallback(() => {
    if (!target || isSamePolicy || !files || files.length === 0) {
      return;
    }

    setSubmitting(true);
    dispatch(
      sendRelocate({
        src: files.map((f) => f.path ?? ""),
        dst_policy_id: target,
      }),
    )
      .then(() => onClose())
      .finally(() => setSubmitting(false));
  }, [dispatch, files, target, isSamePolicy, onClose]);

  return (
    <Dialog open={!!open} onClose={onClose} maxWidth="xs" fullWidth>
      <DialogTitle>{t("application:fileManager.relocation")}</DialogTitle>
      <DialogContent>
        <Box sx={{ mt: 1 }}>
          <DenseSelect
            fullWidth
            value={target}
            onChange={(e) => setTarget(e.target.value as string)}
            disabled={loading}
          >
            {policies.map((p) => (
              <SquareMenuItem key={p.id} value={p.id}>
                {p.name}
              </SquareMenuItem>
            ))}
          </DenseSelect>
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>{t("common:cancel")}</Button>
        <Button
          variant="contained"
          disabled={!target || submitting || isSamePolicy || policies.length === 0}
          onClick={onSubmit}
        >
          {t("application:fileManager.relocation")}
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export default RelocateDialog;
