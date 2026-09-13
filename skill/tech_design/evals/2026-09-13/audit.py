#!/usr/bin/env python3
"""整理真实代理回答并核对证据；不会模拟或重新运行被测模型。"""
import hashlib
import json
import random
import sys
from collections import Counter, defaultdict
from pathlib import Path

ROOT = Path(__file__).resolve().parent


def read(name):
    return json.loads((ROOT / name).read_text())


def write(name, value):
    (ROOT / name).write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n")


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def prepare(manifest):
    # 全部固定尝试均存在后才生成匿名包，避免按表现选择样本。
    packets = []
    redactions = read("anonymization.json")
    traces = {}
    for trial in manifest["trials"]:
        turns = []
        for turn in range(1, manifest["cases"][trial["case"]] + 1):
            relative = f"runs/{trial['id']}/turn-{turn}.md"
            response = (ROOT / relative).read_text()
            assert response.strip(), relative
            traces[relative] = digest(ROOT / relative)
            # 只移除显式自报加载 Skill 的元说明，避免匿名评分直接得知条件。
            # 原始回答不变；具体删除片段另存，任何技术过程都不能匿名化掉。
            for fragment in redactions.get(relative, []):
                assert response.count(fragment) == 1, (relative, fragment)
                response = response.replace(fragment, "", 1)
            turns.append({"turn": turn, "user": (ROOT / f"inputs/{trial['case']}.turn-{turn}.txt").read_text(), "assistant": response})
        packets.append({"trial": trial, "turns": turns})

    random.Random(913).shuffle(packets)
    mapping = {}
    groups = {1: [], 2: []}
    readers = []
    (ROOT / "reading").mkdir(exist_ok=True)
    for number, item in enumerate(packets, 1):
        identifier = f"D{number:02}"
        trial = item["trial"]
        mapping[identifier] = trial
        groups[trial["repetition"]].append({"blind_id": identifier, "case": trial["case"], "turns": item["turns"]})
        if trial["repetition"] == 1:
            # 每份盲读输入只包含最终一条自然回答，不带题目或条件。
            reader_id = f"R{len(readers) + 1:02}"
            write(f"reading/{reader_id}.input.json", {"id": reader_id, "text": item["turns"][-1]["assistant"]})
            readers.append({"reader_id": reader_id, "blind_id": identifier, "trial_id": trial["id"], "turn": item["turns"][-1]["turn"]})
    for number, items in groups.items():
        write(f"grading/packet-{number}.json", {"trials": items})
    write("condition-map.json", mapping)
    write("reader-map.json", readers)
    write("trace-hashes.json", traces)
    print(f"Prepared {len(packets)} trials, {len(traces)} responses, {len(readers)} readers")


def check(manifest):
    # 哈希保护本轮预先固定的版本、题目和判据；引用检查只证明引用存在，
    # 不能证明模型评分的工程判断一定正确。
    for relative, expected in manifest["frozen_sha256"].items():
        assert digest(ROOT / relative) == expected, f"Frozen file changed: {relative}"
    assert digest(ROOT / "skill.snapshot.md") == digest(ROOT.parent.parent / "SKILL.md")
    for relative, expected in read("trace-hashes.json").items():
        assert digest(ROOT / relative) == expected, f"Response changed: {relative}"
    for relative, expected in read("analysis-hashes.json").items():
        assert digest(ROOT / relative) == expected, f"Anonymous scoring input changed: {relative}"
    expected = read("grading/calibration-expected.json")
    for number in (1, 2):
        actual = {item["id"]: item["verdict"] for item in read(f"grading/calibration-{number}.json")["items"]}
        assert actual == expected

    mapping = read("condition-map.json")
    dimensions = {"readability", "focus", "economy", "alternatives", "adaptation", "proactive_interaction", "probed_interaction", "collaboration"}
    counts = defaultdict(lambda: defaultdict(Counter))
    interactions = defaultdict(lambda: defaultdict(Counter))
    checked_quotes = 0
    seen = set()
    rows = []
    for number in (1, 2):
        packet = {item["blind_id"]: item for item in read(f"grading/packet-{number}.json")["trials"]}
        grades = read(f"grading/grades-{number}.json")["trials"]
        assert {item["blind_id"] for item in grades} == set(packet)
        for grade in grades:
            identifier = grade["blind_id"]
            assert identifier not in seen
            seen.add(identifier)
            assert grade["case"] == packet[identifier]["case"]
            assert set(grade["dimensions"]) == dimensions
            responses = {item["turn"]: item["assistant"] for item in packet[identifier]["turns"]}
            for dimension, result in grade["dimensions"].items():
                assert result["verdict"] in {"PASS", "PARTIAL", "FAIL", "NA"}
                assert result["reason"]
                assert result["verdict"] == "NA" or result["evidence"]
                counts[dimension][mapping[identifier]["condition"]][result["verdict"]] += 1
                for quote in result.get("evidence", []):
                    original = (ROOT / f"runs/{mapping[identifier]['id']}/turn-{quote['turn']}.md").read_text()
                    assert quote["quote"] and quote["quote"] in responses[quote["turn"]] and quote["quote"] in original, (identifier, dimension, quote)
                    checked_quotes += 1
            if grade["case"] == "export":
                assert len(grade["interaction_rows"]) == 3
                for result in grade["interaction_rows"]:
                    assert result["first_discovery"] in {"active", "mechanism_only", "missing", "wrong"}
                    assert result["first_guard"] in {"effective", "incomplete", "wrong"}
                    assert result["second_result"] in {"correct", "partial", "wrong"}
                    assert result["second_change"] in {"already_present", "elaboration", "repair"}
                    interactions[result["id"]][mapping[identifier]["condition"]][result["first_discovery"]] += 1
                    for quote in result["evidence"]:
                        original = (ROOT / f"runs/{mapping[identifier]['id']}/turn-{quote['turn']}.md").read_text()
                        assert quote["quote"] and quote["quote"] in responses[quote["turn"]] and quote["quote"] in original, (identifier, result["id"], quote)
                        checked_quotes += 1
            for category in ("strengths", "concrete_issues"):
                for item in grade[category]:
                    quotes = [item] if "quote" in item else item.get("evidence", [])
                    for quote in quotes:
                        original = (ROOT / f"runs/{mapping[identifier]['id']}/turn-{quote['turn']}.md").read_text()
                        assert quote["quote"] and quote["quote"] in responses[quote["turn"]] and quote["quote"] in original, (identifier, category, quote)
                        checked_quotes += 1
            rows.append({**mapping[identifier], "blind_id": identifier, "dimensions": {key: value["verdict"] for key, value in grade["dimensions"].items()}})
    assert len(seen) == len(manifest["trials"])

    # 盲读答案不评分；这里只核对其引用是否真的来自被读的单份文本。
    def check_reader(value, source):
        nonlocal checked_quotes
        if isinstance(value, dict):
            for key, child in value.items():
                if key == "quotes":
                    for quote in child:
                        assert isinstance(quote, str) and quote and quote in source, quote
                        checked_quotes += 1
                else:
                    check_reader(child, source)
        elif isinstance(value, list):
            for child in value:
                check_reader(child, source)

    for item in read("reader-map.json"):
        identifier = item["reader_id"]
        result = read(f"reading/{identifier}.result.json")
        assert result["id"] == identifier
        assert {"operation", "concepts", "decision", "gaps"}.issubset(result)
        check_reader(result, read(f"reading/{identifier}.input.json")["text"])

    lengths = []
    for trial in manifest["trials"]:
        sizes = [len((ROOT / f"runs/{trial['id']}/turn-{turn}.md").read_text()) for turn in range(1, manifest["cases"][trial["case"]] + 1)]
        lengths.append({**trial, "characters_per_turn": sizes, "total_characters": sum(sizes)})
    write("summary.json", {"dimensions": counts, "first_turn_interactions": interactions, "trials": sorted(rows, key=lambda item: item["id"]), "lengths": lengths, "verified_quotes": checked_quotes, "note": "Per-dimension descriptive observations only; no aggregate winner or human comprehension claim"})
    print(f"PASS: frozen source/inputs/rubric, 24 response hashes, calibration 12/12, 12 grades, 6 reader artifacts, {checked_quotes} exact quotes")


if __name__ == "__main__":
    action = sys.argv[1] if len(sys.argv) == 2 else ""
    assert action in {"prepare", "check"}, "Usage: python3 audit.py prepare|check"
    globals()[action](read("manifest.json"))
