import { outsideRepo } from '../../../scripts/bootstrap/lib/runtime.mjs';
import assert from 'node:assert/strict';
import { chromium } from '@playwright/test';
const origin=process.env.ORCH_WORKLOAD_URL,dir=process.env.ORCH_TEST_RUN_DIR;
assert(origin&&dir,'ORCH_WORKLOAD_URL and ORCH_TEST_RUN_DIR required');
outsideRepo(dir);
const browser=await chromium.launch({headless:true});
const context=await browser.newContext({viewport:{width:1440,height:1000}}),page=await context.newPage();
try{
 const response=await page.goto(origin);assert(response.ok(),'Shop HTTP response');
 await page.locator('a[href^="/product/"]').first().click();
 await page.getByRole('button',{name:/Add to Cart/i}).click();
 await page.waitForURL('**/cart');
 assert((await page.locator('body').innerText()).match(/cart/i),'Cart rendered');
 await page.screenshot({path:dir+'/shop-cart.png',fullPage:true});
 console.log('Cart displayed; completing checkout with demo form values');
 const email=page.locator('input[name="email"]');if(await email.count())await email.fill('boutique-test@example.com');
 const year=page.locator('input[name="credit_card_expiration_year"]');if(await year.count())await year.fill('2030');
 await page.getByRole('button',{name:/Place Order/i}).click();
 await page.waitForURL('**/cart/checkout');
 await page.getByText(/Your order is complete/i).waitFor();
 await page.screenshot({path:dir+'/shop-checkout.png',fullPage:true});
 console.log('PASS product → add cart → checkout through browser',page.url());
}catch(e){await page.screenshot({path:dir+'/shop-error.png',fullPage:true});console.error(e.message);process.exitCode=1}finally{await browser.close()}
