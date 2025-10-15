// 覆写文件

// 自定义规则
const customRules = [
];

// 订阅节点
const subscriptions = {
    "sub1" : "sub url",
};

// 自定义代理节点
const customProxies = [
]

function transformProxiesConfig() {
    return [
        {
            url: "https://ghfast.top/https://raw.githubusercontent.com/Keywos/rule/main/rename.js#bl=true&blkey=%E5%AE%B6%E5%AE%BD%2BIPLC%2BIEPL%2B%E8%90%BD%E5%9C%B0%2BIPv6&flag=true&in=true&nm=true&out=en%2Bcn",
        },
    ];
}
function transformBypassConfig() {
    return [
        "127.0.0.1/8",
        "192.168.0.0/16",
        "10.0.0.0/8",
        "172.16.0.0/12",
        "localhost",
        "*.local",
        "*.crashlytics.com",
        "<local>",
        "*.ddev.site",
        "*.neocraftstudio.com",
        "*.ema.games",
        "*.emagames.cn",
        "*.adeagleeye.com",
        "captive.apple.com",
    ]
}


// 国内DNS服务器
const domesticNameservers = [
    "https://223.5.5.5/dns-query", // 阿里DoH
    "https://doh.pub/dns-query", // 腾讯DoH，因腾讯云即将关闭免费版IP访问，故用域名
];
// 国外DNS服务器
const foreignNameservers = [
    "https://cloudflare-dns.com/dns-query", // CloudflareDNS
    "https://77.88.8.8/dns-query", // YandexDNS
    "https://8.8.4.4/dns-query#ecs=1.1.1.1/24&ecs-override=true", // GoogleDNS
    "https://208.67.222.222/dns-query#ecs=1.1.1.1/24&ecs-override=true", // OpenDNS
    "https://9.9.9.9/dns-query", // Quad9DNS
];

// DNS配置
const dnsConfig = {
    "enable": true, // 启用DNS
    "listen": '0.0.0.0:1053',
    "ipv6": true, // 支持IPv6
    "prefer-h3": true, // 优先使用HTTP/3
    "respect-rules": true, // 遵循规则
    // "use-system-hosts": false, // 不使用系统hosts文件
    "cache-algorithm": "arc", // 缓存算法
    "enhanced-mode": "fake-ip", // 增强模式：伪IP
    "fake-ip-range": "198.18.0.1/16", // 伪IP地址范围
    "default-nameserver": ["223.5.5.5", "1.2.4.8"],
    "nameserver": ["system", ...domesticNameservers],
    "proxy-server-nameserver": [...foreignNameservers],
    "direct-nameserver": ["system", ...domesticNameservers],
    "direct-nameserver-follow-policy": false, // 直连DNS不遵循策略
    "nameserver-policy": {
        "geosite:geolocation-!cn":foreignNameservers,
    },
};

function main(params) {
    params.proxies = params.proxies || [];
    // 处理订阅
    processProxyProviders(params)
    // 加上自定义代理
    params.proxies.push(...customProxies);
    if (params.proxies.length === 0 && params["proxy-providers"].length === 0) return params;
    overwriteBasicOptions(params);
    overwriteDns(params);
    overwriteFakeIpFilter(params);
    overwriteHosts(params);
    overwriteTunnel(params);
    overwriteProxyGroups(params);
    overwriteRules(params);
    return params;
}

// 覆写Basic Options
function overwriteBasicOptions(params) {
    const otherOptions = {
        "mixed-port": 7890,
        "allow-lan": false,
        mode: "rule",
        "log-level": "warning",
        ipv6: false,
        "find-process-mode": "strict",
        profile: {
            "store-selected": true,
            "store-fake-ip": true,
        },
        "unified-delay": true,
        "tcp-concurrent": true,
        "global-client-fingerprint": "chrome",
        sniffer: {
            enable: true,
            sniff: {
                HTTP: {
                    ports: [80, "8080-8880"],
                    "override-destination": true,
                },
                TLS: {
                    ports: [443, 8443],
                },
                QUIC: {
                    ports: [443, 8443],
                },
            },
            "skip-domain": ["Mijia Cloud", "+.push.apple.com", "dlg.io.mi.com"],
            "force-domain":["google.com"],
        },
        "geodata-mode": true,
        "geox-url": {
            geosite:"https://gh-proxy.com/https://github.com/v2fly/domain-list-community/releases/latest/download/dlc.dat",
            geoip: "https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/geoip-lite.dat",
            mmdb: "https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/geoip.metadb",
            asn: "https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/GeoLite2-ASN.mmdb",
        },
        "external-controller": "127.0.0.1:9090",
        "external-ui":"ui",
        "external-ui-url":"https://gh-proxy.com/https://github.com/Zephyruso/zashboard/releases/latest/download/dist.zip",
    };
    Object.keys(otherOptions).forEach((key) => {
        params[key] = otherOptions[key];
    });
}

// 覆写DNS
function overwriteDns(params) {
    params.dns = { ...dnsConfig };
}

// 覆写DNS.Fake IP Filter
function overwriteFakeIpFilter (params) {
    params.dns["fake-ip-filter"] = [
        "rule-set:fake_ip_filter",
        // 通用检查
        "geosite:connectivity-check",
        "geosite:private",
    ];
}

// 覆写hosts
function overwriteHosts (params) {
    const hosts = {
        "127.0.0.1.sslip.io": "127.0.0.1",
        "127.atlas.skk.moe": "127.0.0.1",
        "cdn.jsdelivr.net": "cdn.jsdelivr.net.cdn.cloudflare.net",
    };
    params.hosts = hosts;
}

// 覆写Tunnel
function overwriteTunnel(params) {
    const tunnelOptions = {
        enable: false,
        stack: "mixed",
        "dns-hijack": ["any:53", "tcp://any:53"],
        "auto-route": true,
        "auto-detect-interface": true,
        "strict-route": true,
        // 根据自己环境来看要排除哪些网段
        "route-exclude-address": [],
    };
    params.tun = { ...tunnelOptions };
}

// 覆写代理组
function overwriteProxyGroups(params) {
    // 公共的正则片段
    const excludeTerms = "(?i)海外用户|群|邀请|返利|循环|官网|客服|网站|网址|获取|订阅|流量|到期|机场|下次|版本|官址|备用|过期|已用|联系|邮箱|工单|贩卖|通知|倒卖|防止|国内|地址|频道|无法|说明|使用|提示|特别|访问|支持|付费|失联|设置|总计|剩余|主页|游戏|关注|有效|禁止|发布|节点|问题|(\\b(USE|USED|TOTAL|EXPIRE|EMAIL|PANEL)\\b|(\\d{4}-\\d{2}-\\d{2}|\\dG))";
    // 包含条件：各个国家或地区的关键词
    const includeTerms = {
        HK: "香港|HK|Hong|🇭🇰",
        US: "美国|US|United States|America|🇺🇸",

        // 东亚
        TW: "台湾|TW|Taiwan|Wan|🇹🇼|🇨🇳",
        JP: "日本|JP|Japan|🇯🇵",
        KR: "韩国|韓|KR|Korea|🇰🇷",

        // 东南亚
        SG: "新加坡|狮城|SG|Singapore|🇸🇬",
        MY: "马来西亚|大马|MY|Malaysia|🇲🇾",
        VN: "越南|Vietnam|VN|🇻🇳",
        PH: "菲律宾|PH|Philippines|🇵🇭",
        ID: "印尼|印度尼西亚|Indonesia|ID|🇮🇩",
        MM: "缅甸|Myanmar|MM|🇲🇲",
        KH: "柬埔寨|Cambodia|KH|🇰🇭",
        BN: "文莱|Brunei|BN|🇧🇳",
        TL: "东帝汶|Timor-Leste|TL|🇹🇱",
        TH: "泰国|TH|Thailand|🇹🇭",
        LA: "老挝|\\bL\\bA|Laos|🇱🇦",

        // 欧洲
        UK: "英国|UK|United Kingdom|🇬🇧",
        FR: "法国|FR|France|🇫🇷",
        DE: "德国|DE|Germany|🇩🇪",
        NL: "荷兰|Netherlands|NL|🇳🇱",
        ES: "西班牙|Spain|ES|🇪🇸",
        SE: "瑞典|Sweden|SE|🇸🇪",
        CH: "瑞士|Switzerland|CH|🇨🇭",
        PL: "波兰|Poland|\\bP\\bL|🇵🇱",
        IT: "意大利|IT|Italy|🇮🇹",
        RU: "俄罗斯|RU|Russia|🇷🇺",

        // 美洲
        CA: "加拿大|CA|Canada|🇨🇦",
        BR: "巴西|BR|Brazil|🇧🇷",
        AR: "阿根廷|AR|Argentina|🇦🇷",
        MX: "墨西哥|MX|Mexico|🇲🇽",

        // 大洋洲
        AU: "澳大利亚|AU|Australia|🇦🇺",
        NZ: "新西兰|NZ|New Zealand|🇳🇿",

        // 非洲
        ZA: "南非|ZA|South Africa|🇿🇦",
        EG: "埃及|EG|Egypt|🇪🇬",
        NG: "尼日利亚|NG|Nigeria|🇳🇬",
    };

    const ASTerms = Object.values([ includeTerms.TW,
        includeTerms.JP,includeTerms.KR,includeTerms.SG,includeTerms.MY,includeTerms.TH,
        includeTerms.VN,includeTerms.PH,includeTerms.ID,includeTerms.MM,includeTerms.KH,
        includeTerms.BN,includeTerms.TL,
        includeTerms.LA,
    ]).join("|");

    const EUTerms = Object.values([
        includeTerms.UK,includeTerms.FR,includeTerms.DE,includeTerms.NL,includeTerms.ES,
        includeTerms.SE,includeTerms.CH,includeTerms.PL,
        includeTerms.CA,includeTerms.BR,includeTerms.AR,includeTerms.MX,
        includeTerms.RU,includeTerms.IT,
    ]).join("|");

    const OCTerms = Object.values([includeTerms.AU,includeTerms.NZ,]).join("|");

    // 自动代理组正则表达式配置
    const autoProxyGroupRegexs = [
        { name: "🇭🇰 香港", filter : `(?i)(${includeTerms.HK})` },
        { name: "🇺🇸 美国", filter : `(?i)(${includeTerms.US})` },
        { name: "🌏 亚洲", filter : `(?i)(${ASTerms})` },
        { name: "🇪🇺 欧美", filter: `(?i)(${EUTerms})` },
        { name: "🇦🇺 大洋洲", filter: `(?i)(${OCTerms})` },
    ];
    const regionProxyGroups  = autoProxyGroupRegexs
        .map((item) => ({
            name: item.name  + " 自动选择",
            type: "url-test",
            url: "https://cp.cloudflare.com/generate_204",
            interval: 300,
            "include-all" :true,
            filter: item.filter,
            "exclude-filter": excludeTerms,
            proxies: [],
            hidden: true,
        }));

    const autoProxyGroups = autoProxyGroupRegexs
        .map((item) => ({
            name: item.name,
            type: "select",
            "include-all" :true,
            filter: item.filter,
            "exclude-filter": excludeTerms,
            proxies: [item.name  + " 自动选择"],
        }));


    const groups = [
        {
            name: "🎯 节点选择",
            type: "select",
            icon: "https://raw.githubusercontent.com/Orz-3/mini/master/Color/Static.png",
            proxies: [
                "自动选择",
                // 自动选择
                ...autoProxyGroups.map((item) => item.name),
                "DIRECT",
                "⚖️ 负载均衡",
            ],
        },
        {
            name: "自动选择",
            type: "url-test",
            url: "https://cp.cloudflare.com/generate_204",
            interval: 300,
            "include-all" :true,
            "exclude-filter": excludeTerms,
            hidden: true,
        },
        {
            name: "⚖️ 负载均衡",
            type: "load-balance",
            url: "https://cp.cloudflare.com/generate_204",
            interval: 300,
            strategy: "consistent-hashing",
            "include-all" :true,
            "exclude-filter": excludeTerms,
            icon: "https://raw.githubusercontent.com/Orz-3/mini/master/Color/Available.png",
            hidden: true,
        },
        {
            name: "🤖 AIGC",
            type: "select",
            proxies: ["🇺🇸 美国", "🎯 节点选择", ...autoProxyGroups.map((item) => item.name).filter((name) => name !== "🇺🇸 美国")],
            icon: "https://raw.githubusercontent.com/Orz-3/mini/master/Color/OpenAI.png"
        },
        {
            name: "🛑 广告拦截",
            type: "select",
            proxies: ["PASS", "REJECT"],
            icon: "https://raw.githubusercontent.com/Orz-3/mini/master/Color/Adblock.png"
        },
        {
            name: "❓ 其他端口",
            type: "select",
            proxies: ["DIRECT", "🎯 节点选择", "PASS"],
            icon: "https://raw.githubusercontent.com/Orz-3/mini/master/Color/Enet.png"
        },
        {
            name: "🐟 漏网之鱼",
            type: "select",
            proxies: ["🎯 节点选择", "DIRECT", ...autoProxyGroups.map((item) => item.name)],
            icon: "https://raw.githubusercontent.com/Orz-3/mini/master/Color/Fastfish.png"
        },
    ];

    groups.push(...regionProxyGroups);
    groups.push(...autoProxyGroups);

    // 自定义节点
    const allCustomProxies = [
        ...customProxies,
        ...params["proxies"].filter((item) => ["自定义", "🏴"].some(flag => item.name.includes(flag))),
    ]
    if(allCustomProxies.length > 0) {
        groups[0].proxies.push("🏴 自定义节点");
        groups.push({
            name: "🏴 自定义节点",
            type: "select",
            proxies: allCustomProxies.map((item) => item.name),
            icon: "https://raw.githubusercontent.com/Orz-3/mini/master/Color/OvO.png"
        });
    }

    params["proxy-groups"] = groups;
}

// 规则集通用配置
const textRuleProviderCommon = {
    "type": "http", // 规则类型
    "interval": 86400 // 更新间隔（秒），优化为 4 小时更新一次
};
const ruleProviders = {
    fake_ip_filter: {
        ...textRuleProviderCommon,
        format: "text", // 规则格式
        behavior: "domain",
        url: "https://cdn.jsdelivr.net/gh/juewuy/ShellCrash@dev/public/fake_ip_filter.list",
        path: "./rule_set/ShellCrash/fake_ip_filter.list",
        proxy: "🎯 节点选择"
    },
    SteamCN: {
        ...textRuleProviderCommon,
        behavior: "domain",
        url: "https://cdn.jsdelivr.net/gh/blackmatrix7/ios_rule_script@refs/heads/master/rule/Clash/SteamCN/SteamCN.yaml",
        path: "./rule_set/ios_rule_script/SteamCN.yaml",
        proxy: "🎯 节点选择"
    },
    cn: {
        "type": "http", // 规则类型
        "interval": 86400, // 更新间隔（秒），优化为 4 小时更新一次
        behavior: "domain",
        url: "https://cdn.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@meta/geo/geosite/cn.yaml",
        path: "./rule_set/MetaCubeX/cn.yaml",
        proxy: "🎯 节点选择"
    },
};
// 覆写规则
function overwriteRules(params) {
    // GEOSITE查看对应的规则 https://geoviewer.aloxaf.com/
    const Rules = [
        "RULE-SET,cn,DIRECT",
        "RULE-SET,SteamCN,DIRECT",

        // 广告规则
        "GEOSITE,category-ads-all,🛑 广告拦截",
        // 私有域名匹配
        "GEOSITE,private,DIRECT",
        "GEOSITE,category-ai-!cn,🤖 AIGC",
        // BT 公共 Tracker 列表
        "GEOSITE,category-public-tracker,DIRECT",
        // 中国手机号验证码相关服务域名列表
        "GEOSITE,category-number-verification-cn,DIRECT",

        "GEOSITE,geolocation-!cn@cn,DIRECT", // !cn表示国外网站，@cn国内网站。不再国外并且在国内的直连。这一步已经筛选出了国内网站。 例如一些国内加速域名
        "GEOSITE,geolocation-!cn,🎯 节点选择",// 筛选国外网站
        "GEOSITE,geolocation-cn@!cn,🎯 节点选择",// geolocation-cn 整体是国内网站，但存在一些只能在国外访问的域名，那就先使用 @!cn 筛选出这些域名进行代理，剩下的走直连。如 jd.hk 之于 jd 分组
        "GEOSITE,geolocation-cn,DIRECT", // 国内网站
        "GEOSITE,cn,DIRECT",

        // IP 匹配
        "GEOIP,private,DIRECT,no-resolve",
        "GEOIP,telegram,🎯 节点选择",
        "GEOIP,CN,DIRECT",
        "NOT,((DST-PORT,80/443/8080/8888)),❓ 其他端口",

        // 兜底
        "MATCH,🐟 漏网之鱼",
    ];

    params["rule-providers"] = ruleProviders;
    params["rules"] = [
        // 自定义规则
        ...customRules,
        // ip类规则
        ...Rules
    ];
}

// 处理订阅信息
function processProxyProviders(params) {
    let providers = {}
    for (let key in subscriptions) {
        providers[key] = {
            "type": "http", // 订阅类型
            "url": subscriptions[key], // 订阅地址
            "interval": 86400, // 订阅更新时间间隔（秒），优化为 4 小时更新一次
            "health-check": {
                "enable": true,
                "interval": 300,
                "url": "https://cp.cloudflare.com/generate_204"
            },
            "override" :{
                "additional-prefix": `[${key}] `,
            },
        };
    }
    params["proxy-providers"] = providers
}