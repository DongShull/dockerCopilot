#!/bin/sh
cd "${WORKDIR:-/app}" || exit

# 新发布包把二进制放在归档根目录；旧发布包在 dist/linux/<arch>/ 下。
new_bin=""
if [ -f "./dockerCopilot-new" ]; then
    new_bin="./dockerCopilot-new"
else
    for candidate in ./dist/linux/*/dockerCopilot-new; do
        if [ -f "$candidate" ]; then
            new_bin="$candidate"
            break
        fi
    done
fi

if [ -n "$new_bin" ]; then
    # 换入新版本前保留上一版（dockerCopilot.bak），供启动失败时自动回退。
    if [ -f ./dockerCopilot ]; then
        cp ./dockerCopilot ./dockerCopilot.bak
    fi
    mv "$new_bin" ./dockerCopilot
    chmod +x ./dockerCopilot
    # 清掉上一轮回退可能留下的失败版本残留。
    rm -f ./dockerCopilot.failed
fi

# ---- 自更新闭环：新版连续启动失败时自动回退到上一版 ----
FAIL_THRESHOLD=3
FAIL_FILE=./.start-fail-count
child=""

on_term() {
    # docker stop 发来的 SIGTERM 转发给子进程，让它优雅退出。
    if [ -n "$child" ]; then
        kill -TERM "$child" 2>/dev/null
    fi
}
trap on_term TERM INT

while true; do
    start_ts=$(date +%s)
    ./dockerCopilot &
    child=$!
    wait "$child"
    code=$?
    child=""
    elapsed=$(( $(date +%s) - start_ts ))

    # 以下情况不算启动失败：
    #   - 正常退出（0）或被信号终止（143=SIGTERM/137=SIGKILL）；
    #   - 已稳定运行超过 30 秒（自更新的主动退出发生在服务运行良久之后，
    #     崩溃循环则会在启动后几秒内退出）。
    if [ "$elapsed" -ge 30 ] || [ "$code" -eq 0 ] || [ "$code" -eq 143 ] || [ "$code" -eq 137 ]; then
        rm -f "$FAIL_FILE"
        break
    fi

    fails=$(cat "$FAIL_FILE" 2>/dev/null || echo 0)
    fails=$((fails + 1))
    echo "$fails" > "$FAIL_FILE"
    echo "dockerCopilot exited with code $code after ${elapsed}s (fail $fails/$FAIL_THRESHOLD)" >&2

    if [ "$fails" -ge "$FAIL_THRESHOLD" ] && [ -f ./dockerCopilot.bak ]; then
        echo "rolling back to previous binary" >&2
        mv ./dockerCopilot ./dockerCopilot.failed
        mv ./dockerCopilot.bak ./dockerCopilot
        chmod +x ./dockerCopilot
        rm -f "$FAIL_FILE"
    fi
    sleep 2
done
