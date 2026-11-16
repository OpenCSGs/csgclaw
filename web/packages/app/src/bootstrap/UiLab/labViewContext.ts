import { createContext } from "react";

export const LabView = createContext<"all" | "preview" | "design">("all");
