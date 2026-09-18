import { ListItemIcon, ListItemText, Tooltip } from "@mui/material";
import { bindMenu, bindTrigger, usePopupState } from "material-ui-popup-state/hooks";
import { useCallback, useContext, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { getAvailablePolicies, sendPatchPreferredPolicy } from "../../../api/api.ts";
import { AvailableStoragePolicy } from "../../../api/explorer.ts";
import { useAppDispatch, useAppSelector } from "../../../redux/hooks.ts";
import { Filesystem } from "../../../util/uri.ts";
import Checkmark from "../../Icons/Checkmark.tsx";
import StorageOutlined from "../../Icons/StorageOutlined.tsx";
import { SquareMenu, SquareMenuItem } from "../ContextMenu/ContextMenu.tsx";
import { FmIndexContext } from "../FmIndexContext.tsx";
import { ActionButton } from "./TopActions.tsx";

/**
 * Lets the user pick the storage policy used for newly uploaded files in the
 * current folder. The choice is stored on the folder and inherited by its
 * subfolders; the selectable policies are the ones granted to the user's group.
 *
 * The check mark always marks the policy currently in effect: an explicit
 * per-folder preference when one is set, otherwise the group's fallback policy
 * (the first granted one). Picking that same policy explicitly, or any other, is
 * enough to change the behaviour - the backend keeps "no preference" and "the
 * fallback policy" equivalent, so no separate reset action is offered.
 */
const PreferredPolicyPicker = () => {
  const { t } = useTranslation();
  const fmIndex = useContext(FmIndexContext);
  const dispatch = useAppDispatch();
  const path = useAppSelector((s) => s.fileManager[fmIndex].path);
  const fs = useAppSelector((s) => s.fileManager[fmIndex].current_fs);

  const [policies, setPolicies] = useState<AvailableStoragePolicy[]>([]);
  const [preferred, setPreferred] = useState<string | undefined>(undefined);
  const [defaultPolicy, setDefaultPolicy] = useState<string | undefined>(undefined);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);

  const popupState = usePopupState({ variant: "popover", popupId: "preferredPolicy" });

  const reload = useCallback(() => {
    if (!path) {
      return;
    }

    setLoading(true);
    dispatch(getAvailablePolicies({ uri: path }))
      .then((res) => {
        setPolicies(res.policies ?? []);
        setPreferred(res.preferred_policy);
        setDefaultPolicy(res.default_policy);
      })
      .finally(() => {
        setLoading(false);
      });
  }, [dispatch, path]);

  useEffect(() => {
    // Only the user's own file system can carry per-folder preferences.
    if (fs !== Filesystem.my) {
      setPolicies([]);
      return;
    }

    reload();
  }, [reload, fs]);

  const onSelect = useCallback(
    (policyID: string) => {
      if (!path) {
        return;
      }

      setSaving(true);
      dispatch(sendPatchPreferredPolicy({ uri: path, policy_id: policyID }))
        .then(() => {
          setPreferred(policyID);
        })
        .finally(() => {
          setSaving(false);
        });
    },
    [dispatch, path],
  );

  // Policy names are not unique: a policy created by the initial migration is literally
  // named "Default storage policy", so an instance can easily end up with two entries
  // sharing a name. Number the duplicates so the menu stays unambiguous.
  // NOTE: keep every hook above the early return below; hooks must not be skipped.
  const menuPolicies = useMemo(() => {
    const total = new Map<string, number>();
    const seen = new Map<string, number>();
    policies.forEach((policy) => total.set(policy.name, (total.get(policy.name) ?? 0) + 1));

    return policies.map((policy) => {
      if ((total.get(policy.name) ?? 0) < 2) {
        return { policy, label: policy.name };
      }

      const index = (seen.get(policy.name) ?? 0) + 1;
      seen.set(policy.name, index);
      return { policy, label: `${policy.name} (${index})` };
    });
  }, [policies]);

  // Nothing to choose from: the group only grants a single policy.
  if (fs !== Filesystem.my || policies.length <= 1) {
    return null;
  }

  const selected = preferred ?? defaultPolicy;

  return (
    <>
      <Tooltip enterDelay={200} title={t("application:fileManager.storagePolicy")}>
        <ActionButton {...bindTrigger(popupState)} disabled={loading || saving}>
          <StorageOutlined fontSize={"small"} />
        </ActionButton>
      </Tooltip>
      <SquareMenu
        anchorOrigin={{ vertical: "bottom", horizontal: "right" }}
        transformOrigin={{ vertical: "top", horizontal: "right" }}
        MenuListProps={{ dense: true }}
        {...bindMenu(popupState)}
      >
        {menuPolicies.map(({ policy, label }) => (
          <SquareMenuItem key={policy.id} onClick={() => onSelect(policy.id)}>
            <ListItemIcon>{selected === policy.id && <Checkmark fontSize="small" />}</ListItemIcon>
            <ListItemText>{label}</ListItemText>
          </SquareMenuItem>
        ))}
      </SquareMenu>
    </>
  );
};

export default PreferredPolicyPicker;
