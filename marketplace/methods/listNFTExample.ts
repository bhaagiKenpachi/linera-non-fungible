import { listNFT } from './index';
import { readFileSync } from 'fs';


const main = async () => {
    // Sample data for listing the NFT
    const tokenId = "1";
    const price = "0.001"; // Price in Ether

    await listNFT(tokenId, price);
};

main().catch(console.error);