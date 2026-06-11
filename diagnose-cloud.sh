#!/bin/bash
# MediaStationGo 云盘诊断脚本 (Debian/飞牛OS)

echo "=== MediaStationGo 云盘诊断 ==="
echo ""

# 1. 检查容器状态
echo "[1] 检查容器状态..."
docker ps --filter name=mediastation-go --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"
echo ""

# 2. 检查云盘初始化日志
echo "[2] 检查云盘初始化日志..."
docker logs --tail=500 mediastation-go 2>&1 | grep -iE "cloud|storage|boot" | tail -20
echo ""

# 3. 检查CORS配置
echo "[3] 检查CORS配置..."
docker exec mediastation-go cat /data/config.toml 2>/dev/null | grep -i cors
echo ""

# 4. 测试API健康状态
echo "[4] 测试API健康状态..."
if curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/api/health | grep -q 200; then
    echo "API状态: 正常"
else
    echo "API状态: 无法访问"
fi
echo ""

# 5. 检查是否有云盘媒体库
echo "[5] 检查云盘媒体库配置..."
docker logs --tail=1000 mediastation-go 2>&1 | grep -iE "cloud library|ParseCloudLibraryMount" | tail -10
echo ""

echo "=== 诊断完成 ==="
echo ""
echo "说明："
echo "- 如果看到 'no enabled cloud storage configured' -> 需要先配置云盘存储"
echo "- 如果看到 'cloud storage ping failed' -> Cookie过期或网络问题"
echo "- 如果看到 'cloud library scan completed' -> 云盘扫描正常"
