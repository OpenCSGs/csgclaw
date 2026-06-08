import { ComputerDetailPane } from "../ComputerDetailPane";
import type { ComputerDetailPaneProps } from "../ComputerDetailPane";

export function ComputerView(props: ComputerDetailPaneProps) {
  return <ComputerDetailPane {...props} />;
}
