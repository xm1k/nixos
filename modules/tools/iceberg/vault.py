from git.objects.util import verify_utctz
import yaml, sys, git
import pyperclip
from pathlib import Path
from datetime import datetime
from functools import reduce, partial
from collections import deque

class GitManager():
    def __init__(self, repo: Path):
        try: self.repo = git.Repo(repo)
        except: 
            self.repo = git.Repo.init(repo, initial_branch='1.0')
            self.commit(0)

    def commit(self, grade: int):
        self.repo.index.add(["*"])
        try: 
            version = self.repo.head.commit.message.split('.')
            if grade: version[-grade] = str(int(version[-grade]) + 1)
            version = '.'.join(version)
        except: version = '1.0.0'

        self.repo.index.commit(version)

    def go_to(self, version):
        target = next((c.hexsha for c in self.repo.iter_commits() if c.message == version), version)
        self.repo.git.checkout(target)

    def show_tree(self):
        """Выводит в консоль визуальное дерево коммитов и веток."""
        # Используем встроенный git log через GitPython
        # %h - сокращенный хэш, %d - декорации (ветки, теги), %s - сообщение, %cr - дата
        format_str = '%C(magenta)%d%C(reset) %s %C(cyan)(%cr)%C(reset)'
        
        tree_output = self.repo.git.log(
            '--graph', 
            '--all', 
            '--color=always',
            f'--format={format_str}',
            '--abbrev-commit'
        )
        print("\n--- Git Tree Hierarchy ---")
        print(tree_output)
        print("--------------------------\n")

class VaultManager():
    def __init__(self, path: str):
        self.vault_path = Path(path)
        self.git_mgr = GitManager(self.vault_path)

    def _parse_file(self, path, data):
        try:
            parsed = path.read_text(encoding='utf-8').split("---",2)[1:]
            meta, body = yaml.safe_load(parsed[0]), parsed[1]
            last_modified = datetime.fromtimestamp(path.stat().st_mtime).isoformat()

            data["nodes"].append({
                "path": str(path.relative_to(self.vault_path)),
                "tags": meta.get('tags', []),
                "meta": meta,
                "last_updated": last_modified,
                "content": body.strip()
            })

        except:
            path.unlink()
            print('Ошибка в файле:',path,'файл будет удален',file=sys.stderr)
        return data

    def _write_patch(self, node):
        full_path = self.vault_path / node["path"]
        full_path.parent.mkdir(parents=True, exist_ok=True)
        meta = node.get('meta', {})
        if 'tags' in node: meta['tags'] = node['tags']

        meta_block = "---\n" + yaml.dump(meta, allow_unicode=True).strip() + "\n---\n\n"
        full_path.write_text(meta_block + node["content"], encoding='utf-8')

    def _get_version(self, grade):
        try:
            raw = (self.vault_path /'base.yaml').read_text(encoding='utf-8')
            ver = yaml.safe_load(raw)['version'].split('.')
            if grade: ver[-grade] = str(int(ver[-grade]) + 1)
            return '.'.join(ver)
        except:
            return '1.0.0'

    def patch(self):
        data = yaml.safe_load(pyperclip.paste())["nodes"]

        _write_patch_gen = map(self._write_patch, data)
        deque(_write_patch_gen, maxlen=0)

        self.save(1)

    def save(self, grade = 0):
        paths = self.vault_path.rglob('*.md')
        data = reduce(lambda a, x: self._parse_file(x, a), paths, {"nodes": []})

        manifest_file = self.vault_path / "base.yaml"
        with open(manifest_file, 'w', encoding='utf-8') as f:
            yaml.dump(data, f, allow_unicode=True, sort_keys=False)

        if grade: self.git_mgr.commit(grade)

def main():
    vault_mgr = VaultManager('/home/xm1k/Iceberg2')

    commands = {
        "save":  lambda *args: vault_mgr.save(),
        "patch": lambda *args: vault_mgr.patch(),
        "last": lambda *args: vault_mgr.git_mgr.go_head(),
        "go":  lambda *args: vault_mgr.git_mgr.go_to(*args),
        "tree": lambda *args: vault_mgr.git_mgr.show_tree()
    }

    cmd, *args = sys.argv[1:] if len(sys.argv) > 1 else [None]

    action = commands.get(cmd, lambda *_: print(f"Доступные команды: {list(commands.keys())}"))
    action(*args)

if __name__ == '__main__':
    main()

