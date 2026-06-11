package service

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// BootCloudStorageHealthCheck验证所有已配置的云盘存储在启动时是否可用
func (c *Container) BootCloudStorageHealthCheck(ctx context.Context) {
	if c == nil || c.StorageCfg == nil {
		return
	}
	
	configs, err := c.StorageCfg.List(ctx)
	if err != nil {
		c.Log.Warn("boot: cloud storage health check failed to list configs", zap.Error(err))
		return
	}
	
	cloudConfigs := make([]StorageView, 0)
	for _, cfg := range configs {
		if cfg.Enabled && (cfg.Type == "quark" || cfg.Type == "cloud115" || cfg.Type == "clouddrive2" || cfg.Type == "openlist") {
			cloudConfigs = append(cloudConfigs, cfg)
		}
	}
	
	if len(cloudConfigs) == 0 {
		c.Log.Info("boot: no enabled cloud storage configured")
		return
	}
	
	c.Log.Info("boot: checking cloud storage health", zap.Int("count", len(cloudConfigs)))
	
	for _, cfg := range cloudConfigs {
		go func(typ string) {
			checkCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			
			provider, err := c.StorageCfg.CloudProvider(checkCtx, typ)
			if err != nil {
				c.Log.Warn("boot: cloud storage unavailable", zap.String("type", typ), zap.Error(err))
				return
			}
			
			if err := provider.Ping(checkCtx); err != nil {
				c.Log.Warn("boot: cloud storage ping failed", zap.String("type", typ), zap.Error(err))
			} else {
				c.Log.Info("boot: cloud storage healthy", zap.String("type", typ))
			}
		}(cfg.Type)
	}
}
