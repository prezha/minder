"use strict";(self.webpackChunkminder_docs=self.webpackChunkminder_docs||[]).push([[5201],{83257:(e,n,i)=>{i.r(n),i.d(n,{assets:()=>c,contentTitle:()=>o,default:()=>h,frontMatter:()=>l,metadata:()=>t,toc:()=>a});const t=JSON.parse('{"id":"run_minder_server/config_oauth","title":"Create a GitHub OAuth Application","description":"Prerequisites","source":"@site/docs/run_minder_server/config_oauth.md","sourceDirName":"run_minder_server","slug":"/run_minder_server/config_oauth","permalink":"/run_minder_server/config_oauth","draft":false,"unlisted":false,"tags":[],"version":"current","sidebarPosition":120,"frontMatter":{"title":"Create a GitHub OAuth Application","sidebar_position":120},"sidebar":"minder","previous":{"title":"Installing a Production version","permalink":"/run_minder_server/installing_minder"},"next":{"title":"minder","permalink":"/ref/cli/minder"}}');var r=i(74848),s=i(28453);const l={title:"Create a GitHub OAuth Application",sidebar_position:120},o=void 0,c={},a=[{value:"Prerequisites",id:"prerequisites",level:2},{value:"Steps",id:"steps",level:2}];function d(e){const n={a:"a",code:"code",h2:"h2",img:"img",li:"li",ol:"ol",p:"p",ul:"ul",...(0,s.R)(),...e.components};return(0,r.jsxs)(r.Fragment,{children:[(0,r.jsx)(n.h2,{id:"prerequisites",children:"Prerequisites"}),"\n",(0,r.jsxs)(n.ul,{children:["\n",(0,r.jsxs)(n.li,{children:[(0,r.jsx)(n.a,{href:"https://github.com",children:"GitHub"})," account"]}),"\n"]}),"\n",(0,r.jsx)(n.h2,{id:"steps",children:"Steps"}),"\n",(0,r.jsx)(n.p,{children:"A legacy method for allowing users to enroll into Minder is using a GitHub OAuth application."}),"\n",(0,r.jsxs)(n.ol,{children:["\n",(0,r.jsxs)(n.li,{children:["Navigate to ",(0,r.jsx)(n.a,{href:"https://github.com/settings/profile",children:"GitHub Developer Settings"})]}),"\n",(0,r.jsx)(n.li,{children:'Select "Developer Settings" from the left hand menu'}),"\n",(0,r.jsx)(n.li,{children:'Select "OAuth Apps" from the left hand menu'}),"\n",(0,r.jsx)(n.li,{children:'Select "New OAuth App"'}),"\n",(0,r.jsxs)(n.li,{children:["Enter the following details:","\n",(0,r.jsxs)(n.ul,{children:["\n",(0,r.jsxs)(n.li,{children:["Application Name: ",(0,r.jsx)(n.code,{children:"Minder"})," (or any other name you like)"]}),"\n",(0,r.jsxs)(n.li,{children:["Homepage URL: ",(0,r.jsx)(n.code,{children:"http://localhost:8080"})]}),"\n",(0,r.jsxs)(n.li,{children:["Authorization callback URL: ",(0,r.jsx)(n.code,{children:"http://localhost:8080/api/v1/auth/callback/github"})]}),"\n",(0,r.jsxs)(n.li,{children:["If you are prompted to enter a ",(0,r.jsx)(n.code,{children:"Webhook URL"}),", deselect the ",(0,r.jsx)(n.code,{children:"Active"})," option in the ",(0,r.jsx)(n.code,{children:"Webhook"})," section."]}),"\n"]}),"\n"]}),"\n",(0,r.jsx)(n.li,{children:'Select "Register Application"'}),"\n",(0,r.jsx)(n.li,{children:"Generate a client secret"}),"\n",(0,r.jsxs)(n.li,{children:['Copy the "Client ID" , "Client Secret" and "Authorization callback URL" values\ninto your ',(0,r.jsx)(n.code,{children:"./server-config.yaml"})," file, under the ",(0,r.jsx)(n.code,{children:"github"})," section."]}),"\n"]}),"\n",(0,r.jsx)(n.p,{children:(0,r.jsx)(n.img,{alt:"github oauth2 page",src:i(25342).A+"",width:"1282",height:"2402"})})]})}function h(e={}){const{wrapper:n}={...(0,s.R)(),...e.components};return n?(0,r.jsx)(n,{...e,children:(0,r.jsx)(d,{...e})}):d(e)}},25342:(e,n,i)=>{i.d(n,{A:()=>t});const t=i.p+"assets/images/minder-server-oauth-202262cad7bcd33bd0856f08a3cf29a2.png"},28453:(e,n,i)=>{i.d(n,{R:()=>l,x:()=>o});var t=i(96540);const r={},s=t.createContext(r);function l(e){const n=t.useContext(s);return t.useMemo((function(){return"function"==typeof e?e(n):{...n,...e}}),[n,e])}function o(e){let n;return n=e.disableParentContext?"function"==typeof e.components?e.components(r):e.components||r:l(e.components),t.createElement(s.Provider,{value:n},e.children)}}}]);