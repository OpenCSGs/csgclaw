import { useInfiniteQuery } from "@tanstack/react-query";
import { fetchTaskConversations, type TaskConversationPageFilters } from "@/api/taskConversations";

/** The archive count and visible list share the same server-backed cache. */
export function useTaskConversationPages(
  archived: boolean,
  query = "",
  enabled = true,
  filters?: TaskConversationPageFilters,
) {
  return useInfiniteQuery({
    queryKey: filters ? ["task-conversations", archived, query, filters] : ["task-conversations", archived, query],
    enabled,
    initialPageParam: 0,
    queryFn: ({ pageParam }) =>
      filters
        ? fetchTaskConversations(query, archived, pageParam, filters)
        : fetchTaskConversations(query, archived, pageParam),
    getNextPageParam: (last, pages) =>
      last.has_more ? pages.reduce((count, page) => count + page.rooms.length, 0) : undefined,
  });
}
