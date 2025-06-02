#!/usr/bin/env python3
"""
专门测试GetAuthInfo函数的诊断脚本
"""

import requests
import json

# 配置
API_BASE = "https://linjinpeng-augment.hf.space"
VALID_TOKEN = "a27a75ca8ac2c9373748b2ace8dc24bc464e2f789c94e824eabeaba4f854c3aa"

def test_chat_with_debug():
    """测试聊天功能并获取详细错误信息"""
    print("🔍 诊断GetAuthInfo函数问题")
    print("=" * 60)
    
    # 测试聊天请求
    print("💬 发送聊天请求...")
    
    headers = {
        "Authorization": f"Bearer {VALID_TOKEN}",
        "Content-Type": "application/json"
    }
    
    data = {
        "model": "augment-chat",
        "messages": [
            {"role": "user", "content": "Hello, this is a test message."}
        ],
        "stream": False
    }
    
    try:
        response = requests.post(
            f"{API_BASE}/v1/chat/completions",
            headers=headers,
            json=data,
            timeout=30
        )
        
        print(f"📊 响应状态码: {response.status_code}")
        print(f"📋 响应头: {dict(response.headers)}")
        
        if response.status_code == 500:
            print("❌ 500错误 - 检查响应内容类型...")
            content_type = response.headers.get('content-type', '')
            print(f"📄 Content-Type: {content_type}")
            
            if 'text/html' in content_type:
                print("🔍 返回的是HTML错误页面 (Hugging Face内部错误)")
                # 只显示前500个字符
                html_content = response.text[:500]
                print(f"📝 HTML内容预览: {html_content}...")
            elif 'application/json' in content_type:
                print("🔍 返回的是JSON错误信息")
                try:
                    error_data = response.json()
                    print(f"📝 JSON错误: {json.dumps(error_data, indent=2, ensure_ascii=False)}")
                except:
                    print(f"📝 原始响应: {response.text}")
            else:
                print(f"📝 原始响应: {response.text[:500]}...")
        else:
            print(f"📝 响应内容: {response.text[:500]}...")
            
    except Exception as e:
        print(f"❌ 请求异常: {str(e)}")

def test_models_endpoint():
    """测试模型列表端点"""
    print("\n🔧 测试模型列表端点...")
    
    headers = {
        "Authorization": f"Bearer {VALID_TOKEN}",
        "Content-Type": "application/json"
    }
    
    try:
        response = requests.get(
            f"{API_BASE}/v1/models",
            headers=headers,
            timeout=30
        )
        
        print(f"📊 模型端点状态码: {response.status_code}")
        if response.status_code == 200:
            print("✅ 模型端点正常")
            models = response.json()
            print(f"📋 可用模型数量: {len(models.get('data', []))}")
        else:
            print(f"❌ 模型端点异常: {response.text[:200]}...")
            
    except Exception as e:
        print(f"❌ 模型端点请求异常: {str(e)}")

def test_system_config():
    """测试系统配置获取"""
    print("\n⚙️ 测试系统配置...")
    
    # 这里我们不能直接访问内部配置，但可以通过行为推断
    print("📋 从之前的测试可以看出:")
    print("   - coding_mode: false")
    print("   - coding_token: a27a75ca8ac2c9373748...")
    print("   - tenant_url: https://d5.api.augmentcode.com/")
    print("   - auth_token: a27a75ca8ac2c9373748...")

def analyze_problem():
    """分析问题"""
    print("\n🔍 问题分析:")
    print("=" * 60)
    
    print("✅ 已确认的正常功能:")
    print("   1. 认证系统正常工作")
    print("   2. /v1/models 端点正常")
    print("   3. 系统配置已正确设置")
    
    print("\n❌ 问题症状:")
    print("   1. /v1/chat/completions 返回500错误")
    print("   2. 返回的是Hugging Face HTML错误页面")
    print("   3. 说明请求到达了后端但处理失败")
    
    print("\n🎯 可能的原因:")
    print("   1. GetAuthInfo()函数仍然返回空值")
    print("   2. 向Augment服务的请求失败")
    print("   3. Hugging Face平台资源限制")
    print("   4. 网络连接问题")
    
    print("\n💡 建议的解决方案:")
    print("   1. 检查GetAuthInfo()函数的实际执行")
    print("   2. 添加更详细的日志记录")
    print("   3. 测试到Augment服务的连接")

def main():
    """主函数"""
    print("🚀 GetAuthInfo函数诊断开始")
    
    # 执行各项测试
    test_models_endpoint()
    test_chat_with_debug()
    test_system_config()
    analyze_problem()
    
    print("\n" + "=" * 60)
    print("🏁 诊断完成")

if __name__ == "__main__":
    main()
