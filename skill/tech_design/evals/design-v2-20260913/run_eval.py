"""冻结实验、分轮递交输入、归档原文并核对评分证据。

本脚本不生成设计回答，也不判断其正确性。回答由独立上下文的代理生成，
评分由未获知分组的代理完成。所有分组均保留，不选择性排除失败回答。
"""
import hashlib
import json
from pathlib import Path
import random
import shutil
import sys
import tempfile
from collections import Counter, defaultdict

ROOT = Path(__file__).resolve().parent


def read(path):
    return json.loads(path.read_text())


def write(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n")


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def freeze():
    assert not (ROOT / "manifest.json").exists(), "实验已冻结，不可覆盖"
    cases = read(ROOT / "authoring/cases.json")
    trials = []
    # 每个案例每组两次。改变组的递交次序，匿名编号另行打乱。
    orders = [("old", "candidate", "none"), ("none", "old", "candidate")]
    for case in cases:
        for turn, prompt in enumerate(case["turns"], 1):
            target = ROOT / "inputs" / case["id"] / f"turn-{turn}.md"
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text(prompt + "\n")
        for repetition, order in enumerate(orders, 1):
            for condition in order:
                trials.append(dict(id=f"r{len(trials)+1:02}", case=case["id"],
                                   condition=condition, repetition=repetition,
                                   turns=len(case["turns"])))
    anonymous = [f"B{i:02}" for i in range(1, len(trials)+1)]
    random.Random(91326).shuffle(anonymous)
    for trial, blind_id in zip(trials, anonymous):
        trial["blind_id"] = blind_id
    frozen = ["old.snapshot.md", "candidate.snapshot.md", "authoring/cases.json",
              "authoring/blind-risk.md", "authoring/review.md", "grading/rubric.md",
              "grading/calibration.json", "grading/calibration-expected.json",
              "grading/calibration-a.json", "grading/calibration-b.json", "test-design.md"]
    frozen += [str(p.relative_to(ROOT)) for p in sorted((ROOT / "inputs").glob("*/*.md"))]
    manifest = dict(trials=trials, temp_root=tempfile.mkdtemp(prefix="tech-design-v2-"),
                    hashes={p:digest(ROOT / p) for p in frozen},
                    isolation="fresh fork_turns=none; instruction-level shared filesystem; no model override",
                    model="inherited, exact runtime model and sampling metadata unavailable",
                    evidence="design conversation only; model judges, no human comprehension or real side effects",
                    planned_trials=len(trials), planned_responses=sum(t["turns"] for t in trials))
    write(ROOT / "manifest.json", manifest)
    print(json.dumps({k:manifest[k] for k in ("temp_root", "planned_trials", "planned_responses")}))


def stage(trial_id, turn):
    manifest = read(ROOT / "manifest.json")
    trial = next(t for t in manifest["trials"] if t["id"] == trial_id)
    target = Path(manifest["temp_root"]) / trial_id
    target.mkdir(exist_ok=True)
    source = ROOT / "inputs" / trial["case"] / f"turn-{turn}.md"
    shutil.copyfile(source, target / "input.md")
    if turn == 1 and trial["condition"] != "none":
        shutil.copyfile(ROOT / f"{trial['condition']}.snapshot.md", target / "guidance.md")
    print(str(target))


def collect():
    manifest = read(ROOT / "manifest.json")
    outputs = {}
    missing = []
    for trial in manifest["trials"]:
        for turn in range(1, trial["turns"]+1):
            source = Path(manifest["temp_root"]) / trial["id"] / f"turn-{turn}.md"
            relative = f"runs/{trial['id']}/turn-{turn}.md"
            target = ROOT / relative
            if source.exists():
                assert source.stat().st_size > 50, str(source)
                target.parent.mkdir(exist_ok=True)
                if target.exists():
                    assert source.read_bytes() == target.read_bytes(), "已有原文发生变化"
                else:
                    shutil.copyfile(source, target)
                outputs[relative] = digest(target)
            else:
                missing.append(relative)
    write(ROOT / "output-hashes.json", outputs)
    print(json.dumps(dict(collected=len(outputs), missing_count=len(missing), next_missing=missing[:3])))


def package(case_id):
    manifest = read(ROOT / "manifest.json")
    case = next(c for c in read(ROOT / "authoring/cases.json") if c["id"] == case_id)
    answers = []
    redaction_file = ROOT / "grading/anonymization.json"
    redactions = read(redaction_file) if redaction_file.exists() else []
    for trial in manifest["trials"]:
        if trial["case"] != case_id:
            continue
        turns = []
        for number in range(1, trial["turns"]+1):
            response = (ROOT / f"runs/{trial['id']}/turn-{number}.md").read_text()
            # 只应用显式登记的分组自报删除，不改动原始文件或机制解释。
            for redaction in redactions:
                if redaction["trial"] == trial["id"] and redaction["turn"] == number:
                    assert redaction["text"] in response
                    response = response.replace(redaction["text"], "", 1)
            turns.append(dict(turn=number, user=case["turns"][number-1], response=response))
        answers.append(dict(blind_id=trial["blind_id"], case=case_id, turns=turns))
    answers.sort(key=lambda x:x["blind_id"])
    write(ROOT / "grading" / f"pack-{case_id}.json", dict(case=case, trials=answers))
    print(case_id, len(answers))


def check():
    manifest = read(ROOT / "manifest.json")
    for relative, sha in manifest["hashes"].items():
        assert digest(ROOT / relative) == sha, f"冻结文件变化: {relative}"
    assert digest(ROOT.parent.parent / "SKILL.md") == manifest["hashes"]["candidate.snapshot.md"]
    hashes = read(ROOT / "output-hashes.json")
    for relative, sha in hashes.items():
        assert digest(ROOT / relative) == sha, f"原文变化: {relative}"
    quotes = 0
    grades = 0
    def inspect(value, texts):
        nonlocal quotes
        if isinstance(value, dict):
            if "quote" in value and "turn" in value:
                assert value["quote"] and value["quote"] in texts[value["turn"]], value
                quotes += 1
            for child in value.values():
                inspect(child, texts)
        elif isinstance(value, list):
            for child in value:
                inspect(child, texts)
    seen = set()
    cases = {c["id"]:c for c in read(ROOT / "authoring/cases.json")}
    for file in sorted((ROOT / "grading").glob("grades-*.json")):
        judge = file.stem.split("-")[1]
        for grade in read(file)["trials"]:
            trial = next(t for t in manifest["trials"] if t["blind_id"] == grade["blind_id"])
            key = (judge, grade["blind_id"])
            assert key not in seen, f"重复评分: {key}"
            seen.add(key)
            assert grade["case"] == trial["case"]
            assert {p["id"] for p in grade["probes"]} == {p["id"] for p in cases[trial["case"]]["probes"]}
            assert len(grade["probes"]) == len(cases[trial["case"]]["probes"])
            assert set(grade["dimensions"]) == {"clarity", "focus", "economy", "alternatives", "decisions"}
            for item in grade["probes"] + list(grade["dimensions"].values()):
                assert item["verdict"] in {"PASS", "PARTIAL", "FAIL", "NOT_COVERED", "NA"}
                assert item["reason"].strip()
            texts = {n:(ROOT / f"runs/{trial['id']}/turn-{n}.md").read_text()
                     for n in range(1,trial["turns"]+1)}
            inspect(grade,texts)
            grades += 1
    if "--complete" in sys.argv:
        assert len(hashes) == manifest["planned_responses"]
        assert len(seen) == 2 * manifest["planned_trials"]
        assert {j for j, _ in seen} == {"a", "b"}
    print(json.dumps(dict(frozen_files=len(manifest["hashes"]), responses=len(hashes), grades=grades, exact_quotes=quotes)))


def summarize():
    manifest = read(ROOT / "manifest.json")
    lookup = {t["blind_id"]:t for t in manifest["trials"]}
    counts = defaultdict(Counter)
    paired = defaultdict(dict)
    details = []
    for file in sorted((ROOT / "grading").glob("grades-*.json")):
        judge = file.stem.split("-")[1]
        for grade in read(file)["trials"]:
            trial = lookup[grade["blind_id"]]
            counts[(judge, trial["condition"], "first_turn_discovery")][grade["first_turn"]["discovery"]] += 1
            paired[(grade["blind_id"], "first_turn_discovery")][judge] = grade["first_turn"]["discovery"]
            for item in grade["probes"]:
                counts[(judge, trial["condition"], item["id"])][item["verdict"]] += 1
                paired[(grade["blind_id"],item["id"])][judge] = item["verdict"]
            for dimension, item in grade["dimensions"].items():
                counts[(judge, trial["condition"], dimension)][item["verdict"]] += 1
                paired[(grade["blind_id"],dimension)][judge] = item["verdict"]
            details.append(dict(trial=trial, judge=judge,
                                probes={p["id"]:p["verdict"] for p in grade["probes"]},
                                dimensions={d:p["verdict"] for d,p in grade["dimensions"].items()}))
    disagreements = [dict(blind_id=b,item=i,verdicts=v) for (b,i),v in paired.items()
                     if len(v)==2 and len(set(v.values()))>1]
    write(ROOT / "grading/summary.json", dict(counts=[dict(judge=j,condition=c,item=i,counts=dict(v))
        for (j,c,i),v in sorted(counts.items())], disagreements=disagreements, trials=details))
    print(json.dumps(dict(graded_trajectories=len(details), disagreements=len(disagreements))))


if __name__ == "__main__":
    action = sys.argv[1]
    if action == "freeze": freeze()
    elif action == "stage": stage(sys.argv[2], int(sys.argv[3]))
    elif action == "collect": collect()
    elif action == "package": package(sys.argv[2])
    elif action == "check": check()
    elif action == "summarize": summarize()
    else: raise SystemExit("unknown action")
