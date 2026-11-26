#!/usr/bin/env bash
# W2 states 指纹对比(修正转义:length 不需括号转义)
cd /Volumes/user/cz_code/openGym/.claude/worktrees/upbeat-kirch-7f43a4/frontend
export PGPASSWORD=k7Qx2mR9vT4wY8zB
Q="SELECT user_id, doc->>'_rev' AS rev, updated_at FROM states ORDER BY user_id;"
echo "== local(pulled) states =="
psql -h 127.0.0.1 -U muhe -d muhe -t -A -c "$Q"
echo "== prod states =="
/usr/bin/ssh -p 22222 -o ConnectTimeout=15 root@43.129.201.118 "docker exec muhe-fit-pg-1 psql -U muhe -t -A -c \"$Q\""
