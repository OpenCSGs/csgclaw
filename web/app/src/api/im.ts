// @ts-nocheck
import { del, post } from "@/api/client";

export function sendMessageRequest(payload) {
  return post("api/v1/messages", payload);
}

export function createRoomRequest(payload) {
  return post("api/v1/rooms", payload);
}

export function inviteRoomUsersRequest(payload) {
  return post("api/v1/rooms/invite", payload);
}

export function deleteRoomRequest(roomID) {
  return del(`api/v1/rooms/${encodeURIComponent(roomID)}`);
}

export function joinAgentToRoomRequest(payload) {
  return post("api/v1/im/agents/join", payload);
}

export function createUserRequest(payload) {
  return post("api/v1/users", payload);
}
