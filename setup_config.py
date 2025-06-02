#!/usr/bin/env python3
"""
设置系统配置脚本
用于配置coding_token和tenant_url等必要参数
"""

import requests
import json

# 配置
API_BASE = "https://linjinpeng-augment.hf.space"
ACCESS_PASSWORD = "82562118"  # 管理面板密码

def login():
    """登录获取会话token"""
    print("🔐 正在登录管理面板...")
    
    try:
        response = requests.post(
            f"{API_BASE}/api/login",
            json={"password": ACCESS_PASSWORD},
            timeout=30
        )
        
        if response.status_code == 200:
            data = response.json()
            if data.get("status") == "success":
                token = data.get("token")
                print("✅ 登录成功！")
                return token
            else:
                print(f"❌ 登录失败: {data.get('error', '未知错误')}")
                return None
        else:
            print(f"❌ 登录失败！状态码: {response.status_code}")
            print(f"错误信息: {response.text}")
            return None
            
    except Exception as e:
        print(f"❌ 登录错误: {str(e)}")
        return None

def get_current_configs(session_token):
    """获取当前系统配置"""
    print("📋 获取当前系统配置...")
    
    try:
        response = requests.get(
            f"{API_BASE}/api/system/configs",
            headers={"X-Auth-Token": session_token},
            timeout=30
        )
        
        if response.status_code == 200:
            data = response.json()
            if data.get("status") == "success":
                configs = data.get("configs", [])
                print(f"✅ 获取到 {len(configs)} 个配置项")
                
                # 显示关键配置
                key_configs = ["coding_mode", "coding_token", "tenant_url", "auth_token"]
                print("\n🔧 关键配置状态:")
                for config in configs:
                    if config["key"] in key_configs:
                        value = config["value"] if config["value"] else "(空)"
                        if config["key"] in ["coding_token", "auth_token"] and config["value"]:
                            value = config["value"][:20] + "..." if len(config["value"]) > 20 else config["value"]
                        print(f"   {config['key']}: {value}")
                
                return configs
            else:
                print(f"❌ 获取配置失败: {data.get('error', '未知错误')}")
                return None
        else:
            print(f"❌ 获取配置失败！状态码: {response.status_code}")
            return None
            
    except Exception as e:
        print(f"❌ 获取配置错误: {str(e)}")
        return None

def update_config(session_token, key, value, description, category):
    """更新系统配置"""
    print(f"🔧 更新配置 {key}...")
    
    try:
        response = requests.post(
            f"{API_BASE}/api/system/config",
            headers={"X-Auth-Token": session_token},
            json={
                "key": key,
                "value": value,
                "description": description,
                "category": category
            },
            timeout=30
        )
        
        if response.status_code == 200:
            data = response.json()
            if data.get("status") == "success":
                print(f"✅ 配置 {key} 更新成功")
                return True
            else:
                print(f"❌ 配置 {key} 更新失败: {data.get('error', '未知错误')}")
                return False
        else:
            print(f"❌ 配置 {key} 更新失败！状态码: {response.status_code}")
            print(f"错误信息: {response.text}")
            return False
            
    except Exception as e:
        print(f"❌ 配置 {key} 更新错误: {str(e)}")
        return False

def main():
    """主函数"""
    print("🚀 Augment2API 配置设置开始")
    print("=" * 60)
    
    # 1. 登录
    session_token = login()
    if not session_token:
        print("💥 无法登录，退出设置")
        return
    
    print()
    
    # 2. 获取当前配置
    current_configs = get_current_configs(session_token)
    if current_configs is None:
        print("💥 无法获取当前配置，退出设置")
        return
    
    print()
    
    # 3. 设置必要的配置
    configs_to_set = [
        {
            "key": "coding_token",
            "value": "a27a75ca8ac2c9373748b2ace8dc24bc464e2f789c94e824eabeaba4f854c3aa",
            "description": "开发模式Token",
            "category": "development"
        },
        {
            "key": "tenant_url", 
            "value": "https://linjinpeng-augment.hf.space/",
            "description": "开发模式租户URL",
            "category": "development"
        }
    ]
    
    success_count = 0
    for config in configs_to_set:
        if update_config(session_token, **config):
            success_count += 1
    
    print()
    print("=" * 60)
    print("📊 配置设置结果:")
    print(f"   成功: {success_count}/{len(configs_to_set)} 项配置")
    
    if success_count == len(configs_to_set):
        print("🎉 所有配置设置成功！")
        print("✅ coding_token 已设置")
        print("✅ tenant_url 已设置")
        print("\n💡 建议重启服务以使配置生效")
    else:
        print("⚠️ 部分配置设置失败，请检查错误信息")
    
    print()
    
    # 4. 再次获取配置确认
    print("🔍 确认配置设置结果...")
    final_configs = get_current_configs(session_token)

if __name__ == "__main__":
    main()
