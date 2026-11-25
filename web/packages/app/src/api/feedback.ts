import { post, request } from "@/api/client";

export type FeedbackResult = {
  success: boolean;
  issue_id?: number;
  issue_url?: string;
};

export type FeedbackStatus = { state: "not_found" | "unknown" } | { state: "submitted"; receipt: FeedbackResult };

export function readFeedbackStatus(requestID: string): Promise<FeedbackStatus> {
  return post("/api/v1/feedback/status", { request_id: requestID });
}

export function submitFeedback(
  description: string,
  images: File[],
  archives: File[],
  requestID: string,
): Promise<FeedbackResult> {
  const body = new FormData();
  body.set("problem_description", description);
  body.set("request_id", requestID);
  for (const image of images) body.append("images", image);
  for (const archive of archives) body.append("archives", archive);
  return request("/api/v1/feedback", { method: "POST", body });
}
