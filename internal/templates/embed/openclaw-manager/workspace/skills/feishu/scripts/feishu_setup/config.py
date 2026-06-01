"""Shared constants for Feishu/Lark registration."""

ONBOARD_ACCOUNTS_URLS = {
    "feishu": "https://accounts.feishu.cn",
    "lark": "https://accounts.larksuite.com",
}
ONBOARD_OPEN_URLS = {
    "feishu": "https://open.feishu.cn",
    "lark": "https://open.larksuite.com",
}
REGISTRATION_PATH = "/oauth/v1/app/registration"
REQUEST_TIMEOUT = 15
API_REQUEST_TIMEOUT = 600
DEFAULT_EXPIRE_SECONDS = 600

# The manager app performs CSGClaw's Feishu group operations. These scopes cover
# creating a group, listing members, and adding configured worker bots to an
# existing group. Feishu tenant admins still need to approve the scopes.
MANAGER_GROUP_SCOPES = [
    "im:chat:create",
    "im:chat:read",
    "im:chat.members:read",
    "im:chat.members:write_only",
]

STATE_DIR_ENV = "CSGCLAW_FEISHU_SETUP_STATE_DIR"
STATE_DIR_NAME = ".feishu"
LEGACY_STATE_DIR_NAME = ".feishu-channel-setup"
CACHE_STATE_DIR_NAME = "csgclaw-feishu"
LEGACY_CACHE_STATE_DIR_NAME = "csgclaw-feishu-channel-setup"
