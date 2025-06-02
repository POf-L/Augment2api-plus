#!/usr/bin/env python3
"""
调试认证问题的脚本
"""

import requests
import json

API_BASE = "https://linjinpeng-augment.hf.space"

def test_no_auth():
    """测试无认证头的请求"""
    print("🧪 测试无认证头...")
    try:
        response = requests.get(f"{API_BASE}/v1/models", timeout=30)
        print(f"状态码: {response.status_code}")
        print(f"响应: {response.text[:200]}...")
        return response.status_code
    except Exception as e:
        print(f"错误: {e}")
        return None

def test_empty_auth():
    """测试空的认证头"""
    print("\n🧪 测试空认证头...")
    try:
        response = requests.get(
            f"{API_BASE}/v1/models",
            headers={"Authorization": "Bearer "},
            timeout=30
        )
        print(f"状态码: {response.status_code}")
        print(f"响应: {response.text[:200]}...")
        return response.status_code
    except Exception as e:
        print(f"错误: {e}")
        return None

def test_invalid_auth():
    """测试无效认证"""
    print("\n🧪 测试无效认证 (test)...")
    try:
        response = requests.get(
            f"{API_BASE}/v1/models",
            headers={"Authorization": "Bearer test"},
            timeout=30
        )
        print(f"状态码: {response.status_code}")
        print(f"响应: {response.text[:200]}...")
        return response.status_code
    except Exception as e:
        print(f"错误: {e}")
        return None

def test_valid_auth():
    """测试有效认证"""
    print("\n🧪 测试有效认证...")
    token = "a27a75ca8ac2c9373748b2ace8dc24bc464e2f789c94e824eabeaba4f854c3aa"
    try:
        response = requests.get(
            f"{API_BASE}/v1/models",
            headers={"Authorization": f"Bearer {token}"},
            timeout=30
        )
        print(f"状态码: {response.status_code}")
        print(f"响应: {response.text[:200]}...")
        return response.status_code
    except Exception as e:
        print(f"错误: {e}")
        return None

def test_config_endpoint():
    """测试配置端点"""
    print("\n🧪 测试配置端点...")
    try:
        response = requests.get(f"{API_BASE}/admin/config", timeout=30)
        print(f"状态码: {response.status_code}")
        if response.status_code == 200:
            data = response.json()
            coding_mode = data.get('coding_mode', 'unknown')
            print(f"当前 coding_mode: {coding_mode}")
        else:
            print(f"响应: {response.text[:200]}...")
        return response.status_code
    except Exception as e:
        print(f"错误: {e}")
        return None

def main():
    print("🔍 Augment2API 认证调试")
    print("=" * 40)
    
    results = []
    
    # 测试各种认证情况
    results.append(("无认证头", test_no_auth()))
    results.append(("空认证头", test_empty_auth()))
    results.append(("无效认证", test_invalid_auth()))
    results.append(("有效认证", test_valid_auth()))
    results.append(("配置检查", test_config_endpoint()))
    
    print("\n" + "=" * 40)
    print("📊 调试结果汇总:")
    for test_name, status_code in results:
        if status_code:
            print(f"   {test_name}: {status_code}")
        else:
            print(f"   {test_name}: 请求失败")
    
    print("\n🔍 分析:")
    print("- 200: 请求成功")
    print("- 401: 认证失败 (预期)")
    print("- 403: 权限不足")
    print("- 500: 服务器错误")
    
    # 分析结果
    no_auth_code = results[0][1]
    invalid_auth_code = results[2][1]
    valid_auth_code = results[3][1]
    
    if no_auth_code == 200 and invalid_auth_code == 200:
        print("\n⚠️ 警告: 认证系统可能被绕过了！")
    elif valid_auth_code == 401:
        print("\n❌ 有效token被拒绝，可能是token格式或配置问题")
    elif valid_auth_code == 200 and invalid_auth_code == 401:
        print("\n✅ 认证系统工作正常")

if __name__ == "__main__":
    main()
