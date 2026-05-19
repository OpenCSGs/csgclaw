import { createRoot } from "react-dom/client";
import { ReactRoot } from "@/bootstrap/ReactRoot";
import "@/shared/styles/globals.css";

createRoot(document.getElementById("root")!).render(<ReactRoot />);
