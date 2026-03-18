#!/usr/bin/env python3
"""Resolve SLS config with independent built-in profiles (default prod)."""

import argparse
import json
import os
import pathlib
import re
import sys

FIELD_MAP = {
    "endpoint": "Endpoint",
    "ak": "AccessKeyID",
    "sk": "AccessKeySecret",
    "project": "Project",
    "logstore": "Logstore",
}

PROFILE_TO_PARAM = {
    "prod": "ProdSlsConfigParam",
    "test": "TestSlsConfigParam",
}

DEFAULT_PROFILES_PATH = (
    pathlib.Path(__file__).resolve().parent.parent / "references" / "sls_profiles.json"
)


def load_profiles(path: pathlib.Path) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def parse_struct_literal(text: str, name: str) -> dict:
    pattern = rf"{name}\s*=\s*SlsConfigParam\s*\{{(?P<body>.*?)\n\s*\}}"
    match = re.search(pattern, text, flags=re.S)
    if not match:
        raise ValueError(f"Cannot find {name} in config file")

    body = match.group("body")
    parsed = {}
    for line in body.splitlines():
        m = re.match(r'\s*(\w+)\s*:\s*"([^"]*)"\s*,?\s*$', line)
        if m:
            parsed[m.group(1)] = m.group(2)
    return parsed


def resolve_config_from_go(path: pathlib.Path, profile: str) -> dict:
    file_text = path.read_text(encoding="utf-8")
    config_name = PROFILE_TO_PARAM[profile]
    return parse_struct_literal(file_text, config_name)


def resolve_config(args: argparse.Namespace) -> tuple[dict, str]:
    profiles = load_profiles(pathlib.Path(args.profiles_file))
    if args.profile not in profiles:
        raise ValueError(f"Profile '{args.profile}' not found in profiles file")

    config = dict(profiles[args.profile])
    source = f"profiles:{args.profiles_file}"

    if args.config_file:
        go_path = pathlib.Path(args.config_file)
        if not go_path.exists():
            raise ValueError(f"config file not found: {go_path}")
        config = resolve_config_from_go(go_path, args.profile)
        source = f"go:{args.config_file}"

    env_map = {
        "Endpoint": "SLS_ENDPOINT",
        "AccessKeyID": "SLS_ACCESS_KEY_ID",
        "AccessKeySecret": "SLS_ACCESS_KEY_SECRET",
        "Project": "SLS_PROJECT",
        "Logstore": "SLS_LOGSTORE",
    }
    for key, env_name in env_map.items():
        env_val = os.environ.get(env_name)
        if env_val:
            config[key] = env_val

    for cli_key, go_key in FIELD_MAP.items():
        cli_val = getattr(args, cli_key)
        if cli_val:
            config[go_key] = cli_val

    return config, source


def to_env_lines(config: dict) -> str:
    return "\n".join(
        [
            f'SLS_ENDPOINT="{config.get("Endpoint", "")}"',
            f'SLS_ACCESS_KEY_ID="{config.get("AccessKeyID", "")}"',
            f'SLS_ACCESS_KEY_SECRET="{config.get("AccessKeySecret", "")}"',
            f'SLS_PROJECT="{config.get("Project", "")}"',
            f'SLS_LOGSTORE="{config.get("Logstore", "")}"',
        ]
    )


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Resolve Alibaba Cloud SLS config from built-in profiles."
    )
    parser.add_argument(
        "--profiles-file",
        default=str(DEFAULT_PROFILES_PATH),
        help="JSON profiles file with prod/test configs",
    )
    parser.add_argument(
        "--config-file",
        help="Optional Go file path containing TestSlsConfigParam/ProdSlsConfigParam",
    )
    parser.add_argument(
        "--profile",
        choices=["prod", "test"],
        default="prod",
        help="Config profile, default is prod",
    )
    parser.add_argument("--format", choices=["json", "env"], default="json")
    parser.add_argument("--endpoint", help="Override Endpoint")
    parser.add_argument("--ak", help="Override AccessKeyID")
    parser.add_argument("--sk", help="Override AccessKeySecret")
    parser.add_argument("--project", help="Override Project")
    parser.add_argument("--logstore", help="Override Logstore")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    try:
        cfg, source = resolve_config(args)
    except Exception as exc:
        print(f"Error: {exc}", file=sys.stderr)
        return 1

    payload = {
        "profile": args.profile,
        "source_param": PROFILE_TO_PARAM[args.profile],
        "source": source,
        "config": cfg,
    }
    if args.format == "json":
        print(json.dumps(payload, ensure_ascii=False, indent=2))
        return 0

    print(to_env_lines(cfg))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
