"""Shared constants for Feishu/Lark registration."""

ONBOARD_ACCOUNTS_URLS = {
    "feishu": "https://accounts.feishu.cn",
    "lark": "https://accounts.larksuite.com",
}
REGISTRATION_PATH = "/oauth/v1/app/registration"
REQUEST_TIMEOUT = 15
API_REQUEST_TIMEOUT = 600
DEFAULT_EXPIRE_SECONDS = 600

STATE_DIR_ENV = "CSGCLAW_FEISHU_SETUP_STATE_DIR"
STATE_DIR_NAME = ".feishu"
LEGACY_STATE_DIR_NAME = ".feishu-channel-setup"
CACHE_STATE_DIR_NAME = "csgclaw-feishu"
LEGACY_CACHE_STATE_DIR_NAME = "csgclaw-feishu-channel-setup"
