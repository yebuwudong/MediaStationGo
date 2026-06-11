# MediaStationGo 部署更新脚本 (飞牛OS)

echo "=== MediaStationGo 更新部署 ==="
echo ""

# 1. 拉取最新代码
echo "[1] 拉取最新代码..."
git pull origin main

# 2. 重新构建镜像
echo "[2] 重新构建 Docker 镜像..."
docker-compose build --no-cache

# 3. 重启容器
echo "[3] 重启容器..."
docker-compose down
docker-compose up -d

# 4. 等待容器启动
echo "[4] 等待容器启动..."
sleep 5

# 5. 查看启动日志
echo "[5] 查看启动日志（云盘相关）..."
docker logs --tail=100 mediastation-go 2>&1 | grep -iE "boot:|cloud|storage"

echo ""
echo "=== 部署完成 ==="
echo "现在运行 ./diagnose-cloud.sh 查看诊断结果"
