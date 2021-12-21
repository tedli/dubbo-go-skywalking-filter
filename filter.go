/*
 * Licensed Materials - Property of tenxcloud.com
 * (C) Copyright 2021 TenxCloud. All Rights Reserved.
 * 2021-12-17 @author lizhen
 */

package dubbo_go_skywalking_filter

import (
	"context"
	"dubbo.apache.org/dubbo-go/v3/common"
	"dubbo.apache.org/dubbo-go/v3/common/constant"
	"dubbo.apache.org/dubbo-go/v3/common/extension"
	v3filter "dubbo.apache.org/dubbo-go/v3/filter"
	"dubbo.apache.org/dubbo-go/v3/protocol"
	"fmt"
	"github.com/SkyAPM/go2sky"
	agentv3 "skywalking.apache.org/repo/goapi/collect/language/agent/v3"
	"strings"
	"time"
)

const (
	SkyWalkingDubboGoConsumerFilter = "skywalking-consumer"
	SkyWalkingDubboGoProviderFilter = "skywalking-provider"
)

func init() {
	extension.SetFilter(SkyWalkingDubboGoConsumerFilter, func() v3filter.Filter { return filter(consumer) })
	extension.SetFilter(SkyWalkingDubboGoProviderFilter, func() v3filter.Filter { return filter(provider) })
}

type filter func(ctx context.Context, invoker protocol.Invoker, invocation protocol.Invocation) protocol.Result

func (invoke filter) Invoke(ctx context.Context, invoker protocol.Invoker, invocation protocol.Invocation) protocol.Result {
	onFirstCall.Do(initAndStartReporting)
	return invoke(ctx, invoker, invocation)
}

func (filter) OnResponse(_ context.Context, result protocol.Result, _ protocol.Invoker, _ protocol.Invocation) protocol.Result {
	return result
}

const (
	groupKey         = "group"
	dubboComponentID = 3
)

func generateOperationName(url *common.URL, invocation protocol.Invocation) string {
	builder := new(strings.Builder)
	if group := url.GetParam(groupKey, ""); group != "" {
		builder.WriteString(group)
		builder.WriteString("/")
	} else if group = getAttachmentStringValue(invocation, constant.GroupKey); group != "" {
		builder.WriteString(group)
		builder.WriteString("/")
	}
	builder.WriteString(strings.Trim(url.Path, "/"))
	builder.WriteString(".")
	builder.WriteString(invocation.MethodName())
	builder.WriteString("(")
	parameterValues := invocation.ParameterValues()
	lastIndex := len(parameterValues) - 1
	for i, parameter := range parameterValues {
		builder.WriteString(parameter.Type().String())
		if i < lastIndex {
			builder.WriteString(",")
		}
	}
	builder.WriteString(")")
	return builder.String()
}

func generateRequestURL(url *common.URL, operationName string) string {
	builder := new(strings.Builder)
	builder.WriteString(url.Protocol)
	builder.WriteString("://")
	builder.WriteString(url.Ip)
	builder.WriteString(":")
	builder.WriteString(url.Port)
	builder.WriteString("/")
	builder.WriteString(operationName)
	return builder.String()
}

func getAttachmentStringValue(invocation protocol.Invocation, key string) string {
	value := invocation.Attachment(key)
	if value == nil {
		return ""
	}
	if str, ok := value.(string); ok {
		return str
	}
	return ""
}

func invoke(isConsumer bool, ctx context.Context, invoker protocol.Invoker, invocation protocol.Invocation) protocol.Result {
	url := invoker.GetURL()
	host := url.Ip
	port := url.Port
	operationName := generateOperationName(url, invocation)
	var span go2sky.Span
	var err error
	var subContext context.Context
	if isConsumer {
		span, err = tracer.CreateExitSpan(
			ctx, operationName, fmt.Sprintf("%s:%s", host, port),
			func(key, value string) error {
				invocation.SetAttachments(key, value)
				return nil
			})
	} else {
		span, subContext, err = tracer.CreateEntrySpan(ctx, operationName, func(key string) (string, error) {
			return getAttachmentStringValue(invocation, key), nil
		})
		if err != nil {
			span.SetPeer(getAttachmentStringValue(invocation, constant.RemoteAddr))
		}
	}
	defer span.End()
	span.Tag(go2sky.TagURL, generateRequestURL(url, operationName))
	span.SetComponent(dubboComponentID)
	span.SetSpanLayer(agentv3.SpanLayer_RPCFramework)
	// TODO: support collect arguments
	var result protocol.Result
	if subContext != nil {
		result = invoker.Invoke(subContext, invocation)
	} else {
		result = invoker.Invoke(ctx, invocation)
	}
	if err := result.Error(); err != nil {
		span.Error(time.Now(), err.Error())
	}
	return result
}

func consumer(ctx context.Context, invoker protocol.Invoker, invocation protocol.Invocation) protocol.Result {
	return invoke(true, ctx, invoker, invocation)
}

func provider(ctx context.Context, invoker protocol.Invoker, invocation protocol.Invocation) protocol.Result {
	return invoke(false, ctx, invoker, invocation)
}
