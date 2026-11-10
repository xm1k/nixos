# work — поднять рабочий контейнер (ansible / molecule / terraform).
#   work [контейнер] [директория] [сценарий] [конфиг]
# Без аргументов всё выбирается через gum. Секреты берутся из agenix
# (/run/agenix/work_env, KEY=VALUE) и уходят в podman через --env-file,
# VAULT_TOKEN — свежий из ~/.vault-token (его обновляет vault-refresh).
# Для terraform секреты стенда достаются из Vault на хосте, как в Jenkins-джобе.

envfile=/run/agenix/work_env

die() { echo "work: $*" >&2; exit 1; }
choose() { local header=$1; shift; gum choose --header "$header" "$@"; }

[ -r "$envfile" ] || die "нет $envfile — сделай nixos-rebuild switch"

# ---------- env (нужен и для Vault на хосте) ----------
if [ -s "$HOME/.vault-token" ]; then
  VAULT_TOKEN=$(tr -d '[:space:]' < "$HOME/.vault-token")
fi
[ -n "${VAULT_TOKEN:-}" ] || echo "work: VAULT_TOKEN пуст — запусти vault-refresh" >&2
export VAULT_TOKEN="${VAULT_TOKEN:-}"
export VAULT_ADDR="${VAULT_ADDR:-https://re-vault.atmosphere.yadro.com}"

# kv v2: vault_kv <путь в kv_storage> <ключ>
vault_kv() {
  [ -n "$VAULT_TOKEN" ] || die "нет VAULT_TOKEN"
  local resp val
  resp=$(curl -sS -m 15 --noproxy '*' -H "X-Vault-Token: $VAULT_TOKEN" \
    "$VAULT_ADDR/v1/kv_storage/data/$1") || die "Vault недоступен (VPN?)"
  val=$(jq -r --arg k "$2" '.data.data[$k] // empty' <<<"$resp")
  [ -n "$val" ] || die "Vault: нет kv_storage/$1 → $2 ($(jq -c '.errors // empty' <<<"$resp"))"
  printf '%s' "$val"
}

# ---------- 1. контейнер ----------
container=${1:-$(choose "Контейнер" ansible molecule terraform)}
[ -n "$container" ] || exit 1

tls=()
extra=()
galaxy=""
galaxy_extra=""
case "$container" in
  ansible)
    kind=ansible
    image=artifactory-nn.atmosphere.yadro.com:443/re-docker-images/ci/jobenv/rei_ansible:2.0.4
    mnt=/rei_ansible_tools
    galaxy=https://galaxy.atmosphere.yadro.com/api
    scenarios=(servers-configure root shell)
    ;;
  molecule)
    kind=ansible
    image=artifactory-nn.atmosphere.yadro.com/re-docker-images/ci/jobenv/rei_ansible_molecule:2.0.2
    mnt=/re-infra-ansible-tools
    galaxy=https://galaxy-nn.atmosphere.yadro.com/api
    galaxy_extra=yes
    tls=(--tls-verify=false)
    scenarios=(shell root)
    ;;
  terraform)
    kind=terraform
    image=artifactory-nn.atmosphere.yadro.com/re-docker-images/ci/jobenv/terraform_automation:2.1.0
    mnt=/work
    tls=(--tls-verify=false)
    extra=(--privileged)
    scenarios=(openstack shell)
    ;;
  *) die "неизвестный контейнер: $container (ansible|molecule|terraform)" ;;
esac

# terraform: в выбранной директории лежат обе репы
tf_code=dovp-monorepo/rei/terraform
tf_cfgs=rei_openstack/terraform
dir_ok() {
  case "$kind" in
    terraform) [ -d "$1/$tf_code/modules" ] && [ -d "$1/$tf_cfgs" ] ;;
    *) return 0 ;;
  esac
}

# ---------- 2. директория ----------
if [ -n "${2:-}" ]; then
  dir=$2
else
  dirs=()
  if [ "$kind" = terraform ] || [ -e .git ]; then
    dir_ok . && dirs+=(.)
  fi
  while IFS= read -r d; do
    dir_ok "$d" && dirs+=("$d")
  done < <(find . -mindepth 1 -maxdepth 1 -type d ! -name '.*' -printf '%f\n' | sort)
  if [ ${#dirs[@]} -eq 0 ]; then
    [ "$kind" = terraform ] && die "в $PWD нет директории, где лежат и $tf_code, и $tf_cfgs"
    die "в $PWD нет директорий"
  fi
  if [ ${#dirs[@]} -eq 1 ] && [ "$kind" = terraform ]; then
    dir=${dirs[0]}
  else
    dir=$(choose "Директория → $mnt" "${dirs[@]}")
    [ -n "$dir" ] || exit 1
  fi
fi
dir=$(realpath -e -- "$dir") || die "нет директории $dir"
dir_ok "$dir" || die "в $dir нет $tf_code/modules и/или $tf_cfgs"

# ---------- 3. сценарий (только те, для которых есть файлы) ----------
sc_req=playbooks/servers/servers-configure/requirements
root_req=()
for f in requirements.txt requirements.yml requirements.yaml; do
  [ -f "$dir/$f" ] && root_req+=("$f")
done

avail=()
for s in "${scenarios[@]}"; do
  case "$s" in
    servers-configure) [ -d "$dir/$sc_req" ] && avail+=("$s") ;;
    root) [ ${#root_req[@]} -gt 0 ] && avail+=("$s") ;;
    openstack) [ -n "$(find "$dir/$tf_cfgs" -maxdepth 1 -name 'openstack_*.yaml' -print -quit)" ] && avail+=("$s") ;;
    *) avail+=("$s") ;;
  esac
done

if [ -n "${3:-}" ]; then
  scenario=$3
  printf '%s\n' "${avail[@]}" | grep -qxF -- "$scenario" \
    || die "сценарий '$scenario' недоступен для $container в $dir (есть: ${avail[*]})"
elif [ ${#avail[@]} -eq 1 ]; then
  scenario=${avail[0]}
else
  scenario=$(choose "Сценарий" "${avail[@]}")
  [ -n "$scenario" ] || exit 1
fi

# ---------- 4. terraform/openstack: стенд + секреты из Vault ----------
cfg=""
if [ "$scenario" = openstack ]; then
  if [ -n "${4:-}" ]; then
    cfg=$4
    [ -f "$dir/$tf_cfgs/$cfg" ] || cfg=openstack_${4}_CFG.yaml
  else
    cfgs=()
    while IFS= read -r f; do cfgs+=("$f"); done \
      < <(find "$dir/$tf_cfgs" -maxdepth 1 -name 'openstack_*.yaml' -printf '%f\n' | sort)
    cfg=$(choose "Стенд" "${cfgs[@]}")
    [ -n "$cfg" ] || exit 1
  fi
  cfg_path=$dir/$tf_cfgs/$cfg
  [ -f "$cfg_path" ] || die "нет $cfg_path"

  os_location=$(yq -r '.openstack.openstack_location' "$cfg_path")
  art_location=$(yq -r '.global.artifactory_location' "$cfg_path")
  art_key=$(yq -r '.global.artifactory_key' "$cfg_path")

  # Пути — как в terraform_openstack_apply_on-merge.jenkinsfile (reiVaultBasePath = Infra)
  TF_VAR_os_password=$(vault_kv "Infra/OpenStack/$os_location/access" admin)
  TF_BACKEND_TOKEN=$(vault_kv "Infra/Artifactory/$art_location" "$art_key")
  export TF_VAR_os_password TF_BACKEND_TOKEN
  extra+=(-e TF_VAR_os_password -e TF_BACKEND_TOKEN)
fi

# ---------- init-скрипт (без секретов, монтируется read-only) ----------
tmp=$(mktemp -d "${XDG_RUNTIME_DIR:-/tmp}/work.XXXXXX")
trap 'rm -rf "$tmp"' EXIT
init=$tmp/init.sh

{
  printf 'WORK_MNT=%q\nWORK_SCENARIO=%q\n' "$mnt" "$scenario"
  if [ "$kind" = ansible ]; then
    printf 'WORK_SC_REQ=%q\n' "$sc_req"
    printf 'export GALAXY_COLLECTION_PATH=%q\n' "$galaxy"
    if [ -n "$galaxy_extra" ]; then
      printf 'export ANSIBLE_GALAXY_SERVER_LIST=yadro ANSIBLE_GALAXY_SERVER_YADRO_URL=%q\n' "$galaxy"
    fi
    cat <<'EOF'
export ARA_API_CLIENT=offline
export ANSIBLE_FORKS=20
export ANSIBLE_PIPELINING=False
export ANSIBLE_SSH_ARGS='-o ControlMaster=no -o ControlPersist=no'
export VAULT_FLUENTBIT_TOKEN="${VAULT_FLUENTBIT_TOKEN:-$VAULT_TOKEN}"
[ -f "$WORK_MNT/ansible.cfg" ] && export ANSIBLE_CONFIG="$WORK_MNT/ansible.cfg"

galaxy_install() {
  ansible-galaxy role install -r "$1" --ignore-certs --force --server "$GALAXY_COLLECTION_PATH"
  ansible-galaxy collection install -r "$1" --ignore-certs --force --server "$GALAXY_COLLECTION_PATH"
}

echo "=== work: сценарий $WORK_SCENARIO ==="
case "$WORK_SCENARIO" in
  servers-configure)
    python3 -m pip install -r "$WORK_SC_REQ/python.txt"
    galaxy_install "$WORK_SC_REQ/ansible.yml"
    ;;
  root)
    [ -f requirements.txt ] && python3 -m pip install -r requirements.txt
    for f in requirements.yml requirements.yaml; do
      [ -f "$f" ] && galaxy_install "$f"
    done
    ;;
esac

if python3 -c 'import ara' 2>/dev/null; then
  export ANSIBLE_CALLBACK_PLUGINS=$(python3 -m ara.setup.callback_plugins)
  export ANSIBLE_ACTION_PLUGINS=$(python3 -m ara.setup.action_plugins)
  export ANSIBLE_LOOKUP_PLUGINS=$(python3 -m ara.setup.lookup_plugins)
fi
export ANSIBLE_STDOUT_CALLBACK=default
unset -f galaxy_install
echo "=== work: окружение готово ==="
EOF
  else
    printf 'WORK_TF_CODE=%q\nWORK_CFG=%q\n' "$mnt/$tf_code" "${cfg:+$mnt/$tf_cfgs/$cfg}"
    cat <<'EOF'
# как в Jenkins: /ci/modules и /ci/tools → монорепа
mkdir -p /ci
ln -sfn "$WORK_TF_CODE/modules" /ci/modules
ln -sfn "$WORK_TF_CODE/tools" /ci/tools

echo "=== work: сценарий $WORK_SCENARIO ==="
if [ "$WORK_SCENARIO" = openstack ]; then
  echo "=== конфиг: $WORK_CFG ==="
  if python3 /ci/tools/tf_vars_generator/generate.py -c "$WORK_CFG" \
    && cd "$WORK_TF_CODE/modules/openstack" \
    && rm -rf .terraform .terraform.lock.hcl \
    && terraform init -plugin-dir=/ci/terraform_plugins -backend-config="token=$TF_BACKEND_TOKEN" -reconfigure; then
    echo "=== work: готово, дальше terraform plan / apply (os_password уже в TF_VAR_os_password) ==="
  else
    echo "=== work: подготовка упала, смотри вывод выше ===" >&2
  fi
fi
EOF
  fi
} > "$init"

echo "work: $container | $dir → $mnt | $scenario${cfg:+ | $cfg}"

podman run -it --rm "${tls[@]}" "${extra[@]}" \
  -u root:root \
  -w "$mnt" \
  -v "$dir:$mnt" \
  -v "$init:/run/work-init.sh:ro" \
  --env-file "$envfile" \
  -e VAULT_TOKEN -e VAULT_ADDR \
  "$image" \
  /bin/bash -c 'source /run/work-init.sh; exec /bin/bash'
