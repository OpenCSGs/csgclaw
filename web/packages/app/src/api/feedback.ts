import { request } from "@/api/client";

export type FeedbackResult = {
  success: boolean;
  issue_id?: number;
  issue_url?: string;
};

export function submitFeedback(description: string, images: File[]): Promise<FeedbackResult> {
  const body = new FormData();
  body.set("problem_description", description);
  for (const image of images) body.append("images", image);
  return request("/api/v1/feedback", { method: "POST", body });
}
