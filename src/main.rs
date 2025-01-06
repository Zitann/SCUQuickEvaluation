use base64::{Engine as _, engine::general_purpose};
use md5::{Digest, Md5};
use regex::Regex;
use reqwest;
use reqwest::header::{HeaderMap, HeaderValue, CONTENT_TYPE};
use reqwest::multipart;
use serde::{Deserialize, Serialize};
use tokio;
use std::io;
use rpassword::read_password;

const CAPTCHA_URL: &str = "http://zhjw.scu.edu.cn/img/captcha.jpg";
const TOKEN_URL: &str = "http://zhjw.scu.edu.cn/login";
const LOGIN_URL: &str = "http://zhjw.scu.edu.cn/j_spring_security_check";
const OCR_URL: &str = "https://duomi.chenyipeng.com/captcha";
const PJ_URL: &str = "http://zhjw.scu.edu.cn/student/teachingAssessment/evaluation/queryAll";

#[derive(Serialize, Deserialize, Debug)]
struct Record {
    ktid: String,
    kcm: String,
    wjbm: String,
}

fn generate_headers() -> HeaderMap {
    let mut headers = HeaderMap::new();
    headers.insert("Accept", HeaderValue::from_static("text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7"));
    headers.insert("Accept-Encoding", HeaderValue::from_static("gzip, deflate"));
    headers.insert(
        "Accept-Language",
        HeaderValue::from_static("zh-CN,zh;q=0.9,en;q=0.8,en-GB;q=0.7,en-US;q=0.6"),
    );
    headers.insert("Cache-Control", HeaderValue::from_static("no-cache"));
    headers.insert(
        "Cookie",
        HeaderValue::from_static("student.urpSoft.cn=aaaMopex9TYVZhRTRd3pz"),
    );
    headers.insert("DNT", HeaderValue::from_static("1"));
    headers.insert("Host", HeaderValue::from_static("zhjw.scu.edu.cn"));
    headers.insert("Pragma", HeaderValue::from_static("no-cache"));
    headers.insert("Proxy-Connection", HeaderValue::from_static("keep-alive"));
    headers.insert("Upgrade-Insecure-Requests", HeaderValue::from_static("1"));
    headers.insert("User-Agent", HeaderValue::from_static("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0"));
    headers.insert(
        CONTENT_TYPE,
        HeaderValue::from_static("application/x-www-form-urlencoded; charset=UTF-8"),
    );
    headers
}

async fn get_token(client: &reqwest::Client, url: &str) -> Result<String, reqwest::Error> {
    let res = client.get(url).send().await?;
    let body = res.text().await?;
    let token =
        Regex::new(r#"<input type="hidden" id="tokenValue" name="tokenValue" value="(.*?)">"#)
            .unwrap()
            .captures(body.as_str())
            .unwrap()
            .get(1)
            .unwrap()
            .as_str()
            .to_string();
    Ok(token)
}

async fn get_captcha(client: &reqwest::Client) -> Result<String, reqwest::Error> {
    let res = client.get(CAPTCHA_URL).send().await?;
    let body = res.bytes().await?;
    let base64_captcha =general_purpose::STANDARD.encode(&body);
    let res = reqwest::Client::new()
        .post(OCR_URL)
        .form(&[
            (
                "base64img",
                &format!("data:image/png;base64,{}", base64_captcha),
            ),
            ("type", &"0".to_string()),
        ])
        .send()
        .await?;
    let body = res.text().await?;
    // 解析json
    let regex = Regex::new(r#"\{"code": 200, "captcha": "(.*?)"\}"#).unwrap();
    let captcha = regex
        .captures(body.as_str())
        .unwrap()
        .get(1)
        .unwrap()
        .as_str();
    Ok(captcha.to_string())
}

async fn login(
    client: &reqwest::Client,
    username: &str,
    password: &str,
) -> Result<String, reqwest::Error> {
    let mut hasher = Md5::new();
    hasher.update(password);
    let result = hasher.finalize();
    let hashed_password = format!("{:x}", result);

    let token = get_token(&client, TOKEN_URL).await.unwrap();
    let captcha = get_captcha(&client).await.unwrap();

    let res = client
        .post(LOGIN_URL)
        .form(&[
            ("j_username", username),
            ("j_password", &hashed_password),
            ("j_captcha", captcha.as_str()),
            ("tokenValue", token.as_str()),
        ])
        .send()
        .await?;
    let body = res.text().await?;
    // 判断body中是否包含“欢迎您”
    let mut regex = Regex::new(r#"欢迎您"#).unwrap();
    if !regex.is_match(body.as_str()) {
        regex = Regex::new(r#"<strong>发生错误！</strong>(.*?)!"#).unwrap();
        let error = regex
            .captures(body.as_str())
            .unwrap()
            .get(1)
            .unwrap()
            .as_str();
        println!("登录失败");
        Ok(error.to_string())
    } else {
        println!("登录成功");
        Ok("登录成功".to_string())
    }
}

async fn get_pj_list(client: &reqwest::Client) -> Result<(), reqwest::Error> {
    let res = client
        .post(PJ_URL)
        .form(&[("pageNum", "1"), ("pageSize", "30"), ("flag", "kt")])
        .send()
        .await?;
    let body = res.json::<serde_json::Value>().await?;
    // 提取并输出 records 部分
    let records: Vec<Record> = if let Some(records) = body["data"]["records"].as_array() {
        let records: Vec<Record> = records
            .iter()
            .filter(|r| r["SFPG"].as_str().unwrap_or("0") != "1")
            .map(|r| {
                serde_json::from_value(
                    serde_json::json!({ "ktid": r["KTID"], "kcm": r["KCM"], "wjbm": r["WJBM"], }),
                )
                .unwrap()
            })
            .collect(); // 输出所需字段的 records 部分
        if records.is_empty() {
            println!("无待评教课程");
        }
        records
    } else {
        println!("无待评教课程");
        vec![]
    };
    // 清空控制台输出
    print!("\x1B[2J\x1B[1;1H");
    // 输出所有记录
    println!(
        "总共有 {} 门待评教课程",
        records.iter().count()
    );
    println!("");
    for (i, record) in records.iter().enumerate() {
        println!("{}. {}", i + 1, record.kcm);
    }
    println!("");
    println!("输入需要评教的课程编号(空格分隔)");
    println!("或输入 a 评教所有课程");
    println!("或输入 0 退出");
    println!("");
    println!("请输入：");

    let mut input = String::new();
    std::io::stdin().read_line(&mut input).unwrap();
    let input = input.trim();
    // 清空控制台输出
    print!("\x1B[2J\x1B[1;1H");
    if input == "0"||input == "" {
        return Ok(());
    } else if input == "a" {
        println!("评教所有课程");
        println!("");
        for record in records.iter() {
            println!("评教课程 {} 中...", record.kcm);
            pj_one(client, record).await.unwrap();
        }
        println!("");
        println!("按回车键退出...");
        let mut input = String::new();
        let _ = io::stdin().read_line(&mut input);
    } else {
        let input: Vec<usize> = input
            .split_whitespace()
            .filter_map(|x| x.parse().ok())
            .collect();
        for i in input {
            if let Some(record) = records.get(i - 1) {
                println!("评教课程 {} 中...", record.kcm);
                pj_one(client, record).await.unwrap();
            } else {
                println!("无效的课程编号: {}", i);
            }
        }
        println!("");
        println!("按回车键退出...");
        let mut input = String::new();
        let _ = io::stdin().read_line(&mut input);
    }
    Ok(())
}

async fn pj_one(client: &reqwest::Client, record: &Record) -> Result<(), reqwest::Error> {
    let url = format!(
        "http://zhjw.scu.edu.cn/student/teachingEvaluation/newEvaluation/evaluation/{}",
        record.ktid
    );
    let res = client.get(url).send().await?;
    let body = res.text().await?;
    let mut token =
        Regex::new(r#"<input type="hidden" id="tokenValue" name="tokenValue" value="(.*?)"/>"#)
            .unwrap()
            .captures(&body)
            .unwrap()
            .get(1)
            .unwrap()
            .as_str()
            .to_string();

    let name_regex = Regex::new(r#" name="(.*?)""#).unwrap();
    let names: Vec<String> = name_regex
        .captures_iter(&body)
        .map(|cap| cap[1].to_string())
        .collect();

    let form = multipart::Form::new()
        .text("tjcs", "0")
        .text("wjbm", record.wjbm.clone())
        .text("ktid", record.ktid.clone())
        .text("tokenValue", token.clone())
        .text(names[10].clone(), "100")
        .text(names[11].clone(), "A_完全符合")
        .text(names[16].clone(), "A_完全同意")
        .text(names[21].clone(), "A_完全同意")
        .text(
            names[26].clone(),
            "A_老师通过综合教务发布了问卷调查并及时改进教学",
        )
        .text(names[30].clone(), "A_任课老师讲课生动")
        .text(names[30].clone(), "B_课堂上开展了有效的研讨互动教学")
        .text(names[30].clone(), "C_课程进度安排合理，详略得当")
        .text(names[30].clone(), "D_课程内容具有前沿性和时代性")
        .text(names[30].clone(), "E_任课老师肯花时间课外跟学生交流")
        .text(
            names[30].clone(),
            "F_任课老师鼓励学生独立思考，注重培养学生创新精神",
        )
        .text(names[30].clone(), "G_提供了丰富且有效的教学资料")
        .text(names[30].clone(), "H_课程考核方式合理")
        .text(names[30].clone(), "I_课程具有挑战性")
        .text(
            names[30].clone(),
            "J_任课老师就实验操作或实践活动的规范性及安全性做了细致要求",
        )
        .text(names[41].clone(), "A_必须是")
        .text(
            names[45].clone(),
            "这门课程的教学效果很好,老师热爱教学,教学方式生动有趣,课程内容丰富且贴合时代特点。",
        )
        .text("compare", "");

    let post_url = format!("http://zhjw.scu.edu.cn/student/teachingAssessment/baseInformation/questionsAdd/doSave?tokenValue={}", token.clone());
    let res = client.post(post_url).multipart(form).send().await?;
    let body = res.json::<serde_json::Value>().await?;
    println!("{}", body);
    token = body["token"].to_string()[1..body["token"].to_string().len() - 1].to_string();
    println!("{}", token);

    let form = multipart::Form::new()
        .text("tjcs", "1")
        .text("wjbm", record.wjbm.clone())
        .text("ktid", record.ktid.clone())
        .text("tokenValue", token.clone())
        .text(names[10].clone(), "100")
        .text(names[11].clone(), "A_完全符合")
        .text(names[16].clone(), "A_完全同意")
        .text(names[21].clone(), "A_完全同意")
        .text(
            names[26].clone(),
            "A_老师通过综合教务发布了问卷调查并及时改进教学",
        )
        .text(names[30].clone(), "A_任课老师讲课生动")
        .text(names[30].clone(), "B_课堂上开展了有效的研讨互动教学")
        .text(names[30].clone(), "C_课程进度安排合理，详略得当")
        .text(names[30].clone(), "D_课程内容具有前沿性和时代性")
        .text(names[30].clone(), "E_任课老师肯花时间课外跟学生交流")
        .text(
            names[30].clone(),
            "F_任课老师鼓励学生独立思考，注重培养学生创新精神",
        )
        .text(names[30].clone(), "G_提供了丰富且有效的教学资料")
        .text(names[30].clone(), "H_课程考核方式合理")
        .text(names[30].clone(), "I_课程具有挑战性")
        .text(
            names[30].clone(),
            "J_任课老师就实验操作或实践活动的规范性及安全性做了细致要求",
        )
        .text(names[41].clone(), "A_必须是")
        .text(
            names[45].clone(),
            "这门课程的教学效果很好,老师热爱教学,教学方式生动有趣,课程内容丰富且贴合时代特点。",
        )
        .text("compare", "");

    let post_url = format!("http://zhjw.scu.edu.cn/student/teachingAssessment/baseInformation/questionsAdd/doSave?tokenValue={}", token.clone());
    let res = client.post(post_url).multipart(form).send().await?;
    let body = res.json::<serde_json::Value>().await?;
    if body["result"].as_str().unwrap() == "ok" {
        println!("评教成功");
    } else {
        println!("评教失败");
    }

    Ok(())
}

#[tokio::main]
async fn main() {
    let headers = generate_headers();
    let client = reqwest::Client::builder()
        .default_headers(headers)
        .build()
        .unwrap();

    println!("四川大学快速评教助手V2.0");
    println!("");
    println!("本程序不会保存您的学号和密码，请您放心使用。");
    println!("本程序将评教默认为最优评价。如果某课程有特殊评教要求，请您登录教务处进行手动评教。");
    println!("");

    let mut username = String::new();
    let password;
    println!("请输入学号：");
    std::io::stdin().read_line(&mut username).unwrap();
    println!("请输入密码(不显示)：");
    password = read_password().unwrap();
    let username = username.trim();
    let password = password.trim();
    let mut flag = false;
    for _ in 0..3 {
        let res = login(&client, username, password).await.unwrap();
        if res == "登录成功" {
            flag = true;
            break;
        } else {
            println!("{}", res);
        }
    }
    if !flag {
        println!("登录失败次数过多，请稍后再试");
        return;
    }
    get_pj_list(&client).await.unwrap();
}
