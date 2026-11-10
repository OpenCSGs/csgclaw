import type { CloudAgent } from "@/models/cloudAgents";

// Test fixture only; production discovery reads the server publication cache.
export const cloudAgentFixture: readonly CloudAgent[] = [
  {
    id: "agt_demo_contract_reviewer",
    relayURL: "http://127.0.0.1:28080",
    connectionID: "test-relay-connection",
    name: "合同条款审阅",
    description: "逐条比对合同文本，标出偏离模板的条款并给出风险等级。",
    owner: "OpenCSG",
    publisher: "AgenticHub",
    sourceAgentID: "local_agent_002",
    visibility: "public",
    status: "online",
    lastSeenAt: "2026-10-30T01:00:00Z",
    avatar: "avatar/pic-2.png",
  },
];
