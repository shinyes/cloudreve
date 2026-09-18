import { Box, FormControl, SelectChangeEvent, Typography } from "@mui/material";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { getStoragePolicyList } from "../../../../api/api";
import { GroupStoragePolicy, StoragePolicy } from "../../../../api/dashboard";
import { useAppDispatch } from "../../../../redux/hooks";
import FacebookCircularProgress from "../../../Common/CircularProgress";
import { DenseSelect, SquareChip } from "../../../Common/StyledComponents";
import { SquareMenuItem } from "../../../FileManager/ContextMenu/ContextMenu";

export interface PolicySelectionInputProps {
  /** Granted policies, in order. The first one is the fallback policy. */
  value: GroupStoragePolicy[];
  onChange: (value: GroupStoragePolicy[]) => void;
}

const PolicySelectionInput = ({ value, onChange }: PolicySelectionInputProps) => {
  const { t } = useTranslation("dashboard");
  const dispatch = useAppDispatch();
  const [policies, setPolicies] = useState<StoragePolicy[]>([]);
  const [loading, setLoading] = useState(false);
  const [policyMap, setPolicyMap] = useState<Record<number, StoragePolicy>>({});

  const handleChange = (event: SelectChangeEvent<unknown>) => {
    const {
      target: { value },
    } = event;
    // MUI reports the multi-select value as an array of raw item values; the
    // items are policy IDs.
    const selected = (Array.isArray(value) ? value : [value])
      .map((v) => Number(v))
      .filter((id) => !Number.isNaN(id) && id > 0);
    onChange(selected.map((id) => ({ id, name: policyMap[id]?.name })));
  };

  useEffect(() => {
    setLoading(true);
    dispatch(getStoragePolicyList({ page: 1, page_size: 1000, order_by: "id", order_direction: "asc" }))
      .then((res) => {
        setPolicies(res.policies);
        setPolicyMap(
          res.policies.reduce(
            (acc, policy) => {
              acc[policy.id] = policy;
              return acc;
            },
            {} as Record<number, StoragePolicy>,
          ),
        );
      })
      .finally(() => {
        setLoading(false);
      });
  }, []);

  return (
    <FormControl fullWidth>
      <DenseSelect
        multiple
        value={value.map((policy) => policy.id)}
        required
        onChange={handleChange}
        sx={{
          minHeight: 39,
        }}
        disabled={loading}
        MenuProps={{
          PaperProps: { sx: { maxWidth: 230 } },
          MenuListProps: {
            sx: {
              "& .MuiMenuItem-root": {
                whiteSpace: "normal",
              },
            },
          },
        }}
        renderValue={(selected: unknown) => (
          <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
            {loading ? (
              <FacebookCircularProgress size={20} sx={{ mt: "1px" }} />
            ) : (
              (Array.isArray(selected) ? selected : [selected])
                .map((id) => Number(id))
                .filter((id) => policyMap[id]?.id)
                .map((id) => <SquareChip size="small" key={id} label={policyMap[id]?.name} />)
            )}
          </Box>
        )}
      >
        {policies.length > 0 &&
          policies.map((policy) => (
            <SquareMenuItem key={policy.id} value={policy.id}>
              <Box
                sx={{
                  display: "flex",
                  flexDirection: "column",
                }}
              >
                <Typography variant={"body2"} fontWeight={600}>
                  {policy.name}
                </Typography>
                <Typography variant={"caption"} color={"textSecondary"}>
                  {t(`policy.${policy.type}`)}
                </Typography>
              </Box>
            </SquareMenuItem>
          ))}
      </DenseSelect>
    </FormControl>
  );
};

export default PolicySelectionInput;
