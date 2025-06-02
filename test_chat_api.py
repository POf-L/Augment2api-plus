#!/usr/bin/env python3
"""
Augment2API 聊天功能测试脚本
测试非开发模式下的API功能是否正常
"""

import requests
import json
import time

# 配置
API_BASE = "https://linjinpeng-augment.hf.space"
# 非开发模式下需要使用实际的token
REAL_TOKEN = "test"
INVALID_TOKEN = "badtest"  # 无效token用于测试认证

def test_connection_with_token(token, token_name):
    """测试基础连接"""
    print(f"🧪 测试基础连接 ({token_name})...")

    try:
        response = requests.get(
            f"{API_BASE}/v1/models",
            headers={
                "Authorization": f"Bearer {token}",
                "Content-Type": "application/json"
            },
            timeout=30
        )

        if response.status_code == 200:
            data = response.json()
            print(f"✅ 连接成功！状态码: {response.status_code}")
            print(f"📋 可用模型: {json.dumps(data, indent=2, ensure_ascii=False)}")
            return True
        else:
            print(f"❌ 连接失败！状态码: {response.status_code}")
            print(f"错误信息: {response.text}")
            return False

    except Exception as e:
        print(f"❌ 连接错误: {str(e)}")
        return False

def test_auth_with_invalid_token():
    """测试无效token的认证"""
    return test_connection_with_token(INVALID_TOKEN, "无效token")

def test_auth_with_valid_token():
    """测试有效token的认证"""
    return test_connection_with_token(REAL_TOKEN, "有效token")

def test_chat_with_token(token, token_name):
    """测试聊天功能"""
    print(f"\n💬 测试聊天功能 ({token_name})...")

    message = "你好，请简单介绍一下你自己"

    try:
        response = requests.post(
            f"{API_BASE}/v1/chat/completions",
            headers={
                "Authorization": f"Bearer {token}",
                "Content-Type": "application/json"
            },
            json={
                "model": "claude-3-5-sonnet-20241022",
                "messages": [
                    {
                        "role": "user",
                        "content": message
                    }
                ],
                "max_tokens": 150,
                "temperature": 0.7
            },
            timeout=60
        )

        if response.status_code == 200:
            data = response.json()
            print(f"✅ 聊天测试成功！状态码: {response.status_code}")
            print(f"👤 用户消息: {message}")

            if 'choices' in data and len(data['choices']) > 0:
                ai_response = data['choices'][0].get('message', {}).get('content', '无响应内容')
                print(f"🤖 AI响应: {ai_response}")
            else:
                print("⚠️ 响应格式异常，无法获取AI回复")

            print(f"📊 完整响应: {json.dumps(data, indent=2, ensure_ascii=False)}")
            return True
        else:
            print(f"❌ 聊天测试失败！状态码: {response.status_code}")
            print(f"错误信息: {response.text}")
            return False

    except Exception as e:
        print(f"❌ 聊天测试错误: {str(e)}")
        return False

def test_chat_with_invalid_token():
    """测试无效token的聊天"""
    return test_chat_with_token(INVALID_TOKEN, "无效token")

def test_chat_with_valid_token():
    """测试有效token的聊天"""
    return test_chat_with_token(REAL_TOKEN, "有效token")

def main():
    """主测试函数"""
    print("🚀 Augment2API 非开发模式测试开始")
    print("=" * 60)

    # 测试配置信息
    print("🔧 配置信息:")
    print(f"   API端点: {API_BASE}")
    print(f"   开发模式: 已关闭 (false)")
    print(f"   有效Token: {REAL_TOKEN[:20]}...")
    print(f"   无效Token: {INVALID_TOKEN}")
    print()

    # 执行测试
    tests = [
        ("认证测试 - 无效Token", test_auth_with_invalid_token),
        ("认证测试 - 有效Token", test_auth_with_valid_token),
        ("聊天测试 - 无效Token", test_chat_with_invalid_token),
        ("聊天测试 - 有效Token", test_chat_with_valid_token)
    ]

    results = []
    for test_name, test_func in tests:
        print(f"🧪 执行 {test_name}...")
        try:
            result = test_func()
            results.append((test_name, result))
        except Exception as e:
            print(f"❌ {test_name} 执行异常: {str(e)}")
            results.append((test_name, False))

        time.sleep(2)  # 避免请求过快
        print()  # 添加空行分隔

    # 汇总结果
    print("=" * 60)
    print("📊 测试结果汇总:")

    success_count = 0
    expected_failures = ["认证测试 - 无效Token", "聊天测试 - 无效Token"]

    for test_name, result in results:
        if test_name in expected_failures:
            # 对于无效token的测试，失败是预期的
            status = "✅ 预期失败" if not result else "❌ 意外成功"
            expected_result = not result
        else:
            # 对于有效token的测试，成功是预期的
            status = "✅ 成功" if result else "❌ 失败"
            expected_result = result

        print(f"   {test_name}: {status}")
        if expected_result:
            success_count += 1

    print(f"\n🎯 总体结果: {success_count}/{len(results)} 项测试符合预期")

    if success_count == len(results):
        print("🎉 所有测试符合预期！非开发模式下的认证和功能正常！")
        print("✅ 无效token被正确拒绝")
        print("✅ 有效token可以正常使用")
    elif success_count > len(results) // 2:
        print("⚠️ 大部分测试符合预期，但可能存在一些问题。")
    else:
        print("💥 多数测试不符合预期，系统可能存在严重问题。")

if __name__ == "__main__":
    main()
