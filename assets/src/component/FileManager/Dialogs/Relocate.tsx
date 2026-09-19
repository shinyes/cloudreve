import { Alert, Box, Button, Dialog, DialogActions, DialogContent, DialogTitle, Typography } from "@mui/material";
import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { getAvailablePolicies, sendRelocate } from "../../../api/api.ts";
import { AvailableStoragePolicy } from "../../../api/explorer.ts";
import { useAppDispatch, useAppSelector } from "../../../redux/hooks.ts";
import { closeRelocateDialog } from "../../../redux/globalStateSlice.ts";
import { sizeToString } from "../../../util";
import { DenseSelect } from "../../Common/StyledComponents";
import { SquareMenuItem } from "../ContextMenu/ContextMenu.tsx";

/**
 * Moves the data of the selected files and folders to another storage policy.
 *
 * Only the policies granted to the current user's group are offered, and the
 * policies the selection already lives on are excluded: relocating data onto a
 * policy it already uses is a no-op.
 */
const RelocateDialog = () => {
  const { t } = useTranslation();
  const dispatch = useAppDispatch();
  const open = useAppSelector((s) => s.globalState.relocateDialogOpen);
  const files = useAppSelector((s) => s.globalState.relocateDialogFiles);

  const [policies, setPolicies] = useState<AvailableStoragePolicy[]>([]);
  const [target, setTarget] = useState<string>("");
  const [loading, setLoading] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  const currentPolicyIDs = useMemo(
    () => (files ?? []).map((f) => f.extended_info?.storage_policy?.id).filter(Boolean) as string[],
    [files],
  );

  // The policy the selection already lives on is never offered as a target: moving
  // data onto the policy it is already on does nothing. For a mixed selection only the
  // policy that ALL of it already lives on is a no-op, and the rest stay available so
  // the selection can be consolidated onto one of them.
  const candidates = useMemo(
    () =>
      policies.filter((p) => {
        if (currentPolicyIDs.length === 0) {
          return true;
        }
        return !currentPolicyIDs.every((id) => id === p.id);
      }),
    [policies, currentPolicyIDs],
  );

  const totalSize = useMemo(() => (files ?? []).reduce((sum, f) => sum + (f.size ?? 0), 0), [files]);

  // Reported next to the picker so the current location stays visible without being
  // selectable. With a mixed selection the most common policy is shown.
  const currentPolicyName = useMemo(() => {
    const counts = new Map<string, number>();
    for (const id of currentPolicyIDs) {
      counts.set(id, (counts.get(id) ?? 0) + 1);
    }

    let best = "";
    let bestCount = 0;
    for (const [id, count] of counts) {
      if (count > bestCount) {
        best = id;
        bestCount = count;
      }
    }

    return policies.find((p) => p.id === best)?.name ?? "";
  }, [currentPolicyIDs, policies]);

  useEffect(() => {
    if (!open) {
      return;
    }

    setLoading(true);
    dispatch(getAvailablePolicies({}))
      .then((res) => {
        setPolicies(res.policies ?? []);
        // Nothing is pre-selected: every offered policy is a real change of location.
        setTarget("");
      })
      .finally(() => setLoading(false));
  }, [open, dispatch]);

  const onClose = useCallback(() => {
    dispatch(closeRelocateDialog());
  }, [dispatch]);

  const onSubmit = useCallback(() => {
    if (!target || !files || files.length === 0) {
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
  }, [dispatch, files, target, onClose]);

  return (
    <Dialog open={!!open} onClose={onClose} fullWidth maxWidth="sm">
      <DialogTitle>{t("application:fileManager.relocation")}</DialogTitle>
      <DialogContent>
        {candidates.length === 0 && !loading ? (
          <Alert severity="info">{t("application:fileManager.relocateNoTarget")}</Alert>
        ) : (
          <>
            <DenseSelect
              fullWidth
              value={target}
              onChange={(e) => setTarget(e.target.value as string)}
              disabled={loading}
              displayEmpty
            >
              <SquareMenuItem value="">
                <em>{t("application:fileManager.relocateSelectTarget")}</em>
              </SquareMenuItem>
              {candidates.map((p) => (
                <SquareMenuItem key={p.id} value={p.id}>
                  {p.name}
                </SquareMenuItem>
              ))}
            </DenseSelect>
            <Box sx={{ mt: 2 }}>
              {currentPolicyName && (
                <Typography variant="body2" color="text.secondary" sx={{ mb: 0.5 }}>
                  {t("application:fileManager.relocateCurrentPolicy", { policy: currentPolicyName })}
                </Typography>
              )}
              <Typography variant="body2" color="text.secondary">
                {t("application:fileManager.relocateSummary", {
                  count: files?.length ?? 0,
                  size: sizeToString(totalSize),
                })}
              </Typography>
              <Typography variant="caption" color="text.secondary">
                {t("application:fileManager.relocateHint")}
              </Typography>
            </Box>
          </>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>{t("common:cancel")}</Button>
        <Button variant="contained" disabled={!target || submitting || candidates.length === 0} onClick={onSubmit}>
          {t("application:fileManager.relocation")}
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export default RelocateDialog;

