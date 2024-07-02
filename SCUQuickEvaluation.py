import base64
import hashlib
from time import sleep
import requests
import re
import json
import os
from bs4 import BeautifulSoup as bs
from requests_toolbelt.multipart.encoder import MultipartEncoder

class jwc:
    username = input('请输入学号：')
    password = input('请输入密码：')

    captcha_url = "http://zhjw.scu.edu.cn/img/captcha.jpg"
    token_url = "http://zhjw.scu.edu.cn/login" #token地址
    login_url = "http://zhjw.scu.edu.cn/j_spring_security_check"  # 登录接口
    ocr_url = 'https://duomi.chenyipeng.com/captcha'
    score_url = 'http://zhjw.scu.edu.cn/student/integratedQuery/scoreQuery/allTermScores/index'  # 成绩查询接口
    pj_url = 'http://zhjw.scu.edu.cn/student/teachingAssessment/evaluation/queryAll'

    header = {
        'Accept': 'text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.9',
        'Accept-Encoding': 'gzip, deflate',
        'Accept-Language': 'zh-CN,zh;q=0.9',
        'Connection': 'keep-alive',
        'DNT': '1',
        'Host': 'zhjw.scu.edu.cn',
        'Upgrade-Insecure-Requests': '1',
        'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/76.0.3782.0 Safari/537.36 Edg/76.0.152.0',
        'Content-Type': 'application/x-www-form-urlencoded; charset=UTF-8'
    }

    total = 0
    kch = []

    pj= []
    ktid = []
    wjbm = []

    session = requests.session()

    def __init__(self):
        pass
    def get_token(self):
        try:
            response = self.session.get(self.token_url, headers=self.header)
            token = re.findall(r'<input type="hidden" id="tokenValue" name="tokenValue" value="(.*?)">', response.text)[0]
            return token
        except Exception as e:
            print('token获取失败')
            print(e)

    def get_captcha(self):
        try:
            response = self.session.get(self.captcha_url, headers=self.header)
            with open('captcha.jpg', 'wb') as f:
                f.write(response.content)
        except Exception as e:
            print('验证码获取失败')
            print(e)

    def login(self):
        self.get_captcha()
        # 转换为base64
        with open('captcha.jpg', 'rb') as f:
            base64_data = base64.b64encode(f.read())
        
        data = {
            'type': '0',
            'base64img': 'data:image/png;base64,' + base64_data.decode()
        }
        # 忽略证书验证
        response = requests.post(self.ocr_url, data=data)
        text = json.loads(response.text)['captcha']
        print(f'验证码识别结果{text}')

        data = {
            'tokenValue': self.get_token(),
            'j_username': self.username,
            'j_password': hashlib.md5(self.password.encode()).hexdigest(),
            'j_captcha': text
        }
        response = self.session.post(self.login_url, headers=self.header, data=data)
        if "欢迎您" in response.text:
            print("登陆成功！")
            return True
        else:
            print("登陆失败！")
            return False
        

    def get_pj_list(self):
        data = {
            'pageNum': '1',
            'pageSize': '30',
            'flag': 'kt'
        }
        response = self.session.post(self.pj_url, headers=self.header, data=data)
        data = response.json()['data']['records']
        for i in range(len(data)):
            if(data[i]['SFPG']=='0'):
                self.pj.append(data[i]['KCM'])
                self.ktid.append(data[i]['KTID'])
                self.wjbm.append(data[i]['WJBM'])
        if(len(self.pj)==0):
            print('无待评教课程')
        else:
            print(f'总共{len(self.pj)}门待评教课程')
            for i in range(len(self.pj)):
                print(f'{i}.{self.pj[i]}')
            print()
            print(f'{len(self.pj)}.一键全部评教')
            print(f'{len(self.pj)+1}.退出')
            print()
            ready = input('以空格分隔输入待评教课程编号：').strip()
            if(int(ready[0])==len(self.pj)+1):
                return
            elif(int(ready[0])==len(self.pj)):
                for i in range(len(self.pj)):
                    self.pj_one(i)
            else:
                ready = ready.split(' ')
                for i in ready:
                    self.pj_one(int(i))

    def pj_one(self, i):
        url = 'http://zhjw.scu.edu.cn/student/teachingEvaluation/newEvaluation/evaluation/'+self.ktid[i]
        try:
            response = self.session.get(url, headers=self.header)
            soup = bs(response.text, 'html.parser')
            # find table
            table = soup.find('table')
            # get all input name and type
            inputs = table.find_all('input')
            map_form = {}
            map_form['tjcs']='0'
            map_form['wjbm']=soup.find('input', {'name':'wjbm'})['value']
            map_form['ktid']=soup.find('input', {'name':'ktid'})['value']
            map_form['tokenValue']=soup.find('input', {'name':'tokenValue'})['value']
            for input_ in inputs:
                # 获取input所有属性
                attrs = input_.attrs
                
                if 'placeholder' in attrs and input_['placeholder']=='请输入1-100的整数':
                    map_form[input_['name']] = '100'
                elif 'type' in attrs and input_['type'] == 'radio' and map_form.get(input_['name']) == None:
                    map_form[input_['name']] = input_['value']
                elif 'type' in attrs and input_['type'] == 'checkbox':
                    if map_form.get(input_['name']) == None:
                        map_form[input_['name']] = []
                    if input_['value'] == 'K_以上均无':
                        continue
                    map_form[input_['name']].append(input_['value'])
            textarea_name = re.findall(r'<textarea name="(.*?)" class="form-control value_element" style="width:300%;height:60px;" maxlength="500"></textarea>', response.text)[0]
            map_form[textarea_name] = '这门课程的教学效果很好,老师热爱教学,教学方式生动有趣,课程内容丰富且贴合时代特点。'
            map_form['compare'] = ''
            params = {
        **{f"{k}": v_item for k, v in map_form.items() if isinstance(v, list) for  v_item in v},
        **{k: v for k, v in map_form.items() if not isinstance(v, list)}
    }
            data = MultipartEncoder(params, boundary='------WebKitFormBoundaryPt8uDhx6i4giheJk')
            # print(data)
            post_url = 'http://zhjw.scu.edu.cn/student/teachingAssessment/baseInformation/questionsAdd/doSave?tokenValue='+ map_form['tokenValue']
            headers = {
                'Accept': 'application/json, text/javascript, */*; q=0.01',
                'Accept-Language': 'zh-CN,zh;q=0.9,en;q=0.8,en-GB;q=0.7,en-US;q=0.6',
                'DNT': '1',
                'Content-Type': data.content_type,
                'Origin': 'http://zhjw.scu.edu.cn',
                'Proxy-Connection': 'keep-alive',
                'Referer': 'http://zhjw.scu.edu.cn/student/teachingEvaluation/newEvaluation/evaluation/'+self.ktid[i],
                'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0',
                'X-Requested-With': 'XMLHttpRequest',
            }
            # 100秒延迟
            #sleep(110)
            response = self.session.post(post_url, headers=headers, data=data)
            sleep(1)
            params['tjcs']='1'
            params['tokenValue']=response.json()['token']
            data = MultipartEncoder(params, boundary='------WebKitFormBoundaryPt8uDhx6i4giheJk')
            response = self.session.post(post_url, headers=headers, data=data)

            if(response.status_code==200 and response.json()['result']=='ok'):
                print(f'{self.pj[i]}评教完成')
                print(response.text)
            else:
                print(f'{self.pj[i]}评教失败')
                print(response.text)
        except Exception as e:
            print('token获取失败')
            print(e)


if __name__ == '__main__':
    count = 0
    jwc = jwc()
    while(not jwc.login() and count<3):
        count+=1
    if(count==3):
        print('登录失败,请检查学号密码')
        os.system('pause')
        exit()
    sleep(1)
    jwc.get_pj_list()
    jwc.session.close()
    os.remove('captcha.jpg')
    os.system('pause')
