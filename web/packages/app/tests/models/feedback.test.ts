import { feedbackImagesValid, MAX_FEEDBACK_IMAGE_BYTES } from "@/models/feedback";

it("validates image count, size and format", () => {
  const image = { size: 100, type: "image/png" };
  expect(feedbackImagesValid(Array(6).fill(image))).toBe(true);
  expect(feedbackImagesValid(Array(7).fill(image))).toBe(false);
  expect(feedbackImagesValid([{ ...image, size: 0 }])).toBe(false);
  expect(feedbackImagesValid([{ ...image, size: MAX_FEEDBACK_IMAGE_BYTES + 1 }])).toBe(false);
  expect(feedbackImagesValid([{ ...image, type: "image/svg+xml" }])).toBe(false);
});
