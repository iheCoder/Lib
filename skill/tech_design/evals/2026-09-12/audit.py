#!/usr/bin/env python3
"""整理真实对话并检查评分证据；不会重新调用模型，也不按关键词评价设计质量。"""
from pathlib import Path
import argparse
import hashlib
import json
import random

ROOT = Path(__file__).resolve().parent
CASES = {'cloud': 3, 'finance': 4, 'simple': 1}
DIMENSIONS = ('interaction', 'models', 'process', 'boundary', 'revision', 'convergence')


def read_json(path):
    return json.loads(path.read_text())


def write_json(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n')


def prepare():
    # 原文只在同一试次内拼接；评分包隐藏条件，保留每轮用户消息及回复。
    trials = [f'{case}-{condition}{repeat}' for case in CASES
              for condition in ('s', 'b') for repeat in (1, 2)]
    random.Random(20260912).shuffle(trials)
    packets, mapping, hashes = [], {}, {}
    for index, trial in enumerate(trials, 1):
        case = trial.split('-')[0]
        turns = []
        for turn in range(1, CASES[case] + 1):
            path = ROOT / 'runs' / trial / f'turn-{turn}.md'
            if not path.exists() or not path.read_text().strip():
                raise ValueError(f'缺少真实回复：{path}')
            hashes[str(path.relative_to(ROOT))] = hashlib.sha256(path.read_bytes()).hexdigest()
            turns.append({'turn': turn,
                          'user': (ROOT / 'inputs' / f'{case}.turn-{turn}.txt').read_text(),
                          'assistant': path.read_text()})
        alias = f'Q{index:02d}'
        mapping[alias] = trial
        packets.append({'trial': alias, 'case': case, 'turns': turns})
    # 条件映射独立保存，评分者只获得 packet 文件。
    write_json(ROOT / 'condition-map.json', mapping)
    for group in (1, 2):
        write_json(ROOT / 'grading' / f'packet-{group}.json', packets[(group-1)*6:group*6])
    write_json(ROOT / 'trace-hashes.json', hashes)
    print(f'已整理 {len(packets)} 个试次、{len(hashes)} 个真实回复')


def check():
    # 文件和引用检查可机械验证，开放质量判断保留给独立评分者。
    manifest = read_json(ROOT / 'manifest.json')
    snapshot = hashlib.sha256((ROOT / 'skill.snapshot.md').read_bytes()).hexdigest()
    current = hashlib.sha256((ROOT.parent.parent / 'SKILL.md').read_bytes()).hexdigest()
    assert snapshot == current == manifest['skill_sha256'], '被测 Skill 发生变化'
    for name, digest in manifest['input_sha256'].items():
        assert hashlib.sha256((ROOT / 'inputs' / name).read_bytes()).hexdigest() == digest, name
    for name, digest in read_json(ROOT / 'trace-hashes.json').items():
        assert hashlib.sha256((ROOT / name).read_bytes()).hexdigest() == digest, name
    mapping = read_json(ROOT / 'condition-map.json')
    rows = []
    for group in (1, 2):
        packets = {p['trial']: p for p in read_json(ROOT / 'grading' / f'packet-{group}.json')}
        grades = read_json(ROOT / 'grading' / f'grades-{group}.json')
        assert {g['trial'] for g in grades} == set(packets), '漏评或多评'
        for grade in grades:
            packet = packets[grade['trial']]
            assert set(grade['ratings']) == set(DIMENSIONS), grade['trial']
            for dimension, rating in grade['ratings'].items():
                assert rating['status'] in ('PASS', 'PARTIAL', 'FAIL', 'NA')
                assert rating.get('reason'), (grade['trial'], dimension)
                if rating['status'] != 'NA':
                    assert rating.get('evidence'), (grade['trial'], dimension)
                for evidence in rating.get('evidence', []):
                    text = packet['turns'][evidence['turn']-1]['assistant']
                    assert evidence['quote'] in text, (grade['trial'], dimension, evidence['quote'])
            all_pass = all(v['status'] in ('PASS', 'NA') for v in grade['ratings'].values())
            rows.append({'trial': mapping[grade['trial']], 'anonymous_id': grade['trial'],
                         'full_pass': all_pass,
                         'ratings': {k: v['status'] for k, v in grade['ratings'].items()}})
    counts = {condition: {'complete': 0, 'total': 0} for condition in ('skill', 'baseline')}
    for row in rows:
        condition = 'skill' if row['trial'].split('-')[1].startswith('s') else 'baseline'
        counts[condition]['total'] += 1
        counts[condition]['complete'] += row['full_pass']
    result = {'checks': 'PASS', 'trials': rows, 'observed_counts': counts,
              'scope': '固定题目中的模型评分计数；无真人校准，不是生产可靠性或因果提升估计'}

    # 明确正反例只能校准本次评分，不能算作被测 Skill 的成功试次。
    expected = read_json(ROOT / 'grading' / 'calibration-expected.json')
    result['calibration'] = {}
    for group in (1, 2):
        actual = read_json(ROOT / 'grading' / f'calibration-{group}.json')
        matched = sum(actual[key]['status'] == status for key, status in expected.items())
        result['calibration'][str(group)] = {'matched': matched, 'total': len(expected)}
        assert matched == len(expected), '评分器校准存在未解决分歧'

    # 追加输入与追加成绩单独检查；它们不能改变原始三轮的成绩。
    for name, digest in read_json(ROOT / 'grading' / 'primary-grade-hashes.json').items():
        assert hashlib.sha256((ROOT / 'grading' / name).read_bytes()).hexdigest() == digest
    supplementary = ROOT / 'grading' / 'supplement-grades.json'
    if supplementary.exists():
        spec = read_json(ROOT / 'supplement-manifest.json')
        assert hashlib.sha256((ROOT / 'inputs' / 'cloud.supplement.txt').read_bytes()).hexdigest() == spec['input_sha256']
        for name, digest in read_json(ROOT / 'supplement-hashes.json').items():
            assert hashlib.sha256((ROOT / name).read_bytes()).hexdigest() == digest, name
        packets = {p['trial']: p for p in read_json(ROOT / 'grading' / 'cloud-supplement-packet.json')}
        grades = read_json(supplementary)
        assert {g['trial'] for g in grades} == set(packets)
        for grade in grades:
            assert grade['status'] in ('PASS', 'PARTIAL', 'FAIL')
            assert grade['reason'] and grade['evidence']
            for evidence in grade['evidence']:
                assert evidence['turn'] == 4
                assert evidence['quote'] in packets[grade['trial']]['assistant']
        result['supplement'] = [{'trial': mapping[g['trial']], 'status': g['status']} for g in grades]

    # 独立复核保留为第二份判断；只比较差异，绝不静默覆盖主成绩。
    review_path = ROOT / 'grading' / 'cloud-review.json'
    if review_path.exists():
        packets = {p['trial']: p for p in read_json(ROOT / 'grading' / 'cloud-review-packet.json')}
        primary = {row['anonymous_id']: row['ratings'] for row in rows}
        reviews = read_json(review_path)
        assert {review['trial'] for review in reviews} == set(packets)
        differences = []
        for review in reviews:
            for dimension, rating in review['ratings'].items():
                assert dimension in ('interaction', 'convergence')
                assert rating['status'] in ('PASS', 'PARTIAL', 'FAIL')
                assert rating['reason'] and rating['evidence']
                for evidence in rating['evidence']:
                    assert evidence['quote'] in packets[review['trial']]['turns'][evidence['turn']-1]['assistant']
                original = primary[review['trial']][dimension]
                if original != rating['status']:
                    differences.append({'trial': mapping[review['trial']], 'dimension': dimension,
                                        'primary': original, 'review': rating['status']})
        result['review_disagreements'] = differences
    write_json(ROOT / 'summary.json', result)
    print(json.dumps(result, ensure_ascii=False, indent=2))


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=('prepare', 'check'))
    args = parser.parse_args()
    {'prepare': prepare, 'check': check}[args.action]()
