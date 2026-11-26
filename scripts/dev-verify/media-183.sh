#!/usr/bin/env bash
# media-183.sh — 给 183 测试版补动作演示媒体(catalogue 授权媒体 → dist/exercise-media)
# 用法: bash media-183.sh   (打包+scp+远端解包+curl 验证)
set -euo pipefail
WT=/Volumes/user/cz_code/openGym/.claude/worktrees/upbeat-kirch-7f43a4
SSH="/usr/bin/ssh -i $HOME/.ssh/pve_root_ed25519 -o IdentitiesOnly=yes -F none -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no"
SCP="/usr/bin/scp -i $HOME/.ssh/pve_root_ed25519 -o IdentitiesOnly=yes -F none -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no"

echo "== 1. stage-media 到本地 dist =="
cd "$WT/frontend"
node "$WT/scripts/catalogue/stage-media.mjs" dist
du -sh dist/exercise-media

echo "== 2. 打包 =="
tar -C "$WT/frontend/dist" -czf /tmp/exercise-media.tgz exercise-media
ls -lh /tmp/exercise-media.tgz

echo "== 3. scp 到 183 =="
$SCP /tmp/exercise-media.tgz qm@100.98.98.183:/tmp/

echo "== 4. 远端解包进 dist(nginx 宿主挂载,即解即生效) =="
$SSH qm@100.98.98.183 'cd ~/muhe-open183/frontend/dist && rm -rf exercise-media && tar xzf /tmp/exercise-media.tgz && ls exercise-media && du -sh exercise-media'

echo "== 5. 验证 =="
$SSH qm@100.98.98.183 'curl -s -o /dev/null -w "still %{http_code} %{size_download}B\n" http://127.0.0.1:3131/exercise-media/still/0001.webp; curl -s -o /dev/null -w "clip %{http_code} %{size_download}B\n" http://127.0.0.1:3131/exercise-media/clip/0001.mp4'
echo "DONE"
