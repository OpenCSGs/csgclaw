import { useInfiniteQuery } from "@tanstack/react-query";
import { fetchTaskConversations } from "@/api/taskConversations";

/** The archive count and visible list share the same server-backed cache. */
export function useTaskConversationPages(archived: boolean, query = "", enabled = true) {
  return useInfiniteQuery({
    queryKey: ["task-conversations", archived, query],
    enabled,
    initialPageParam: 0,
    queryFn: ({ pageParam }) => fetchTaskConversations(query, archived, pageParam),
    getNextPageParam: (last, pages) =>
      last.has_more ? pages.reduce((count, page) => count + page.rooms.length, 0) : undefined,
  });
}
