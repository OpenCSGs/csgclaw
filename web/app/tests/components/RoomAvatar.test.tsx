import { render } from "@testing-library/react";
import { RoomAvatar, resolveRoomAvatarMembers } from "@/components/business/RoomAvatar";

const members = [
  { id: "u-1", name: "Alpha", avatar: "" },
  { id: "u-2", name: "Beta", avatar: "" },
  { id: "u-3", name: "Gamma", avatar: "" },
  { id: "u-4", name: "Delta", avatar: "" },
  { id: "u-5", name: "Epsilon", avatar: "" },
];

describe("RoomAvatar", () => {
  it("renders the two-person split layout", () => {
    const { container } = render(<RoomAvatar count={2} members={members.slice(0, 2)} />);

    expect(container.firstChild).toHaveClass("room-avatar--count-2");
    expect(container.querySelectorAll(".room-avatar-tile")).toHaveLength(2);
    expect(container.querySelector(".room-avatar-count-badge")).not.toBeInTheDocument();
    expect(container.querySelector(".room-avatar-tile--left")).toBeInTheDocument();
    expect(container.querySelector(".room-avatar-tile--right")).toBeInTheDocument();
  });

  it("renders the three-person asymmetric layout", () => {
    const { container } = render(<RoomAvatar count={3} members={members.slice(0, 3)} />);

    expect(container.firstChild).toHaveClass("room-avatar--count-3");
    expect(container.querySelectorAll(".room-avatar-tile")).toHaveLength(3);
    expect(container.querySelector(".room-avatar-tile--top-left")).toBeInTheDocument();
    expect(container.querySelector(".room-avatar-tile--bottom-left")).toBeInTheDocument();
    expect(container.querySelector(".room-avatar-tile--tall")).toBeInTheDocument();
  });

  it("renders the four-plus layout with a count badge", () => {
    const { container } = render(<RoomAvatar count={5} members={members} />);

    expect(container.firstChild).toHaveClass("room-avatar--count-4");
    expect(container.querySelectorAll(".room-avatar-tile")).toHaveLength(4);
    expect(container.querySelector(".room-avatar-count-badge")).toHaveTextContent("5");
  });

  it("filters the current user out of room avatar members", () => {
    const conversation = {
      id: "room-1",
      is_direct: false,
      members: ["u-local", "u-2", "u-3"],
      messages: [],
      title: "Room 1",
    };

    expect(resolveRoomAvatarMembers(conversation, new Map(members.map((item) => [item.id, item])), "u-local")).toEqual([
      expect.objectContaining({ id: "u-2" }),
      expect.objectContaining({ id: "u-3" }),
    ]);
  });
});
