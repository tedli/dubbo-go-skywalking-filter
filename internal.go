/*
 * Licensed Materials - Property of tenxcloud.com
 * (C) Copyright 2021 TenxCloud. All Rights Reserved.
 * 2021-12-17 @author lizhen
 */

package dubbo_go_skywalking_filter

import (
	"dubbo.apache.org/dubbo-go/v3/common/extension"
	"dubbo.apache.org/dubbo-go/v3/common/logger"
	"dubbo.apache.org/dubbo-go/v3/config"
	"github.com/SkyAPM/go2sky"
	factory "github.com/SkyAPM/go2sky/reporter"
	"sync"
)

var (
	onFirstCall = new(sync.Once)
	reporter    go2sky.Reporter
	tracer      *go2sky.Tracer
)

const (
	skywalkingConfigTopLevelKey  = "skywalking"
	skywalkingCollectorServerKey = "skywalkingCollectorServer"
	collectorServerKey           = "collectorServer"
)

func getGrpcSetting() (server, application string) {
	application = config.GetApplicationConfig().Name
	// TODO: Support more settings
	obj := config.GetDefineValue(skywalkingConfigTopLevelKey, nil)
	if obj != nil {
		if setting, ok := obj.(map[string]interface{}); ok {
			if collectorServer := setting[collectorServerKey]; collectorServer != nil {
				if value, ok := collectorServer.(string); ok {
					server = value
					return
				}
			}
		}
	}
	collectorServer := config.GetDefineValue(skywalkingCollectorServerKey, "")
	if value, ok := collectorServer.(string); ok {
		server = value
		return
	}
	return
}

// This cannot achieve using init(), because at that time,
// the dubbo-go root config, yet not be parsed, further if
// dobbo-go provides a 'RegisterConfigParsedCallback' liked api
// we can refactor to use that
func initAndStartReporting() {
	server, application := getGrpcSetting()
	var err error
	if server == "" {
		reporter, _ = factory.NewLogReporter()
		logger.Infof("no skywalking collector configured, use log reporter")
	} else if reporter, err = factory.NewGRPCReporter(server); err != nil {
		reporter, _ = factory.NewLogReporter()
		logger.Infof("init skywalking grpc collector failed, use log reporter, server: %s, error: %s", server, err)
	} else {
		logger.Infof("init skywalking grpc collector succeed, server: %s", server)
	}
	extension.AddCustomShutdownCallback(reporter.Close)
	if tracer, err = go2sky.NewTracer(application, go2sky.WithReporter(reporter)); err != nil {
		logger.Warnf("init skywalking tracer failed, error: %s", err)
	}
}
