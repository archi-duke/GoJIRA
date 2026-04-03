
1. 사용자 인증은 SSO 서버를 통해서 인증을 받아야 함.
2. 인증 서버는 인증성공시 해당 클라이언트에게 쿠키로 인증정보를 전달할 것임
3. 인증 서버에 아래와 같이 리다이렉션 하면 인증서버가 SSO 서비스 연계해서 인증을 진행하고 
   인증이 완료되면 인증서버가 클라이언트에게 쿠키를 전달하고 
   클라이언트는 인증서버로 부터 전달받은 쿠키로 인증된 사용자를 확인하여 인증된 사용자의 정보를 화면에 표시함.
   
   let uinfo = this.Cookie.GetCookie('ADSSO_USER');
   if (uinfo === null || uinfo === "") 
   { 
     window.location.href = https://{인증서버 주소:포트}/Account/ADFSLogin?client=${window.location.origin}; 
   }



