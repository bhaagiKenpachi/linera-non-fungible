import { buyNFT } from './index';

const main = async () => {
    const tokenId = "1"; // Replace with a valid token ID
    const pvtKey = "ae1d058b9c5713848e7ac4c1901fac9a737729a34c10c997991c861dd7705ac0";
    await buyNFT(pvtKey, tokenId);
};

main().catch(console.error); 