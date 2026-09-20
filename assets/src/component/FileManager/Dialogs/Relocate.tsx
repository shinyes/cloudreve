import { Alert, Box, Button, Dialog, DialogActions, DialogContent, DialogTitle, Typography } from "@mui/material";
import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { getAvailablePolicies, getFileInfo, sendRelocate } from "../../../api/api.ts";
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

  // The policy each selected file currently lives on is not part of a list response:
  // `extended_info` is only filled in by the single-file detail endpoint. Listing the
  // current policy as a target would therefore be a no-op offer, so the details are
  // fetched when the dialog opens. Loading both arrays with one Promise.all keeps the
  // ids and their names aligned by index.
  const [currentPolicy, setCurrentPolicy] = useState<{ ids: string[]; name: string }>({ ids: [], name: "" });

  useEffect(() => {
    if (!open) {
      return;
    }

    const list = files ?? [];
    if (list.length === 0) {
      setCurrentPolicy({ ids: [], name: "" });
      return;
    }

    let cancelled = false;
    Promise.all(
      list.map((f) =>
        f.path
          ? dispatch(getFileInfo({ uri: f.path, extended: true }))
              .then((res) => res.extended_info?.storage_policy ?? null)
              .catch(() => null)
          : Promise.resolve(null),
      ),
    ).then((infos) => {
      if (cancelled) {
        return;
      }

      const ids = infos.map((info) => info?.id).filter(Boolean) as string[];
      const counts = new Map<string, number>();
      infos.forEach((info, index) => {
        if (info?.id) {
          counts.set(info.id, (counts.get(info.id) ?? 0) + 1);
        }
      });

      // The most common policy describes the selection; a tie keeps the first seen.
      let bestIndex = -1;
      let bestCount = 0;
      infos.forEach((info, index) => {
        if (!info?.id) {
          return;
        }
        const count = counts.get(info.id) ?? 0;
        if (count > bestCount) {
          bestCount = count;
          bestIndex = index;
        }
      });

      setCurrentPolicy({ ids, name: bestIndex >= 0 ? (infos[bestIndex]?.name ?? "") : "" });
    });

    return () => {
      cancelled = true;
    };
  }, [open, files, dispatch]);

  // The policy the selection already lives on is never offered as a target: moving data
  // onto the policy it is already on does nothing. For a mixed selection only the policy
  // that ALL of it already lives on is a no-op, and the rest stay available so the
  // selection can be consolidated onto one of them.
  const candidates = useMemo(
    () =>
      policies.filter((p) => {
        if (currentPolicy.ids.length === 0) {
          return true;
        }
        return !currentPolicy.ids.every((id) => id === p.id);
      }),
    [policies, currentPolicy.ids],
  );

  const totalSize = useMemo(() => (files ?? []).reduce((sum, f) => sum + (f.size ?? 0), 0), [files]);

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
              {currentPolicy.name && (
                <Typography variant="body2" color="text.secondary" sx={{ mb: 0.5 }}>
                  {t("application:fileManager.relocateCurrentPolicy", { policy: currentPolicy.name })}
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

