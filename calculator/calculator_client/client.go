package main

import (
	"context"
	"fmt"
	"go-grpc-intro/calculator/calculatorpb"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/balancer/roundrobin"
	"google.golang.org/grpc/resolver"
	"io"
	"log"
	"time"

	"google.golang.org/grpc/codes"

	"google.golang.org/grpc/status"

	"google.golang.org/grpc"
)

func main() {

	fmt.Println("Calculator Client")
	//cc, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
	//if err != nil {
	//	log.Fatalf("could not connect: %v", err)
	//}
	//defer cc.Close()

	conn, err := GetConnection(context.Background(), "localhost:50051")
	if err != nil {
		log.Println("could not connect to:", "localhost:50051", err)
		return
	}

	defer func() {
		if conn != nil {
			conn.Close()
		}
	}()

	c := calculatorpb.NewCalculatorServiceClient(conn)
	// fmt.Printf("Created client: %f", c)

	//doUnary(c)

	// doServerStreaming(c)

	// doClientStreaming(c)

	doBiDiStreaming(c)

	//doErrorUnary(c)
}

func doUnary(c calculatorpb.CalculatorServiceClient) {
	fmt.Println("Starting to do a Sum Unary RPC...")
	req := &calculatorpb.SumRequest{
		FirstNumber:  5,
		SecondNumber: 40,
	}
	res, err := c.Sum(context.Background(), req)
	if err != nil {
		log.Fatalf("error while calling Sum RPC: %v", err)
	}
	log.Printf("Response from Sum: %v", res.SumResult)
}

func doServerStreaming(c calculatorpb.CalculatorServiceClient) {
	fmt.Println("Starting to do a PrimeDecomposition Server Streaming RPC...")
	req := &calculatorpb.PrimeNumberDecompositionRequest{
		Number: 12390392840,
	}
	stream, err := c.PrimeNumberDecomposition(context.Background(), req)
	if err != nil {
		log.Fatalf("error while calling PrimeDecomposition RPC: %v", err)
	}
	for {
		res, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatalf("Something happened: %v", err)
		}
		fmt.Println(res.GetPrimeFactor())
	}
}

func doClientStreaming(c calculatorpb.CalculatorServiceClient) {
	fmt.Println("Starting to do a ComputeAverage Client Streaming RPC...")

	stream, err := c.ComputeAverage(context.Background())
	if err != nil {
		log.Fatalf("Error while opening stream: %v", err)
	}

	numbers := []int32{3, 5, 9, 54, 23}

	for _, number := range numbers {
		fmt.Printf("Sending number: %v\n", number)
		stream.Send(&calculatorpb.ComputeAverageRequest{
			Number: number,
		})
	}

	res, err := stream.CloseAndRecv()
	if err != nil {
		log.Fatalf("Error while receiving response: %v", err)
	}

	fmt.Printf("The Average is: %v\n", res.GetAverage())
}

func doBiDiStreaming(c calculatorpb.CalculatorServiceClient) {
	fmt.Println("Starting to do a FindMaximum BiDi Streaming RPC...")

	stream, err := c.FindMaximum(context.Background())

	if err != nil {
		log.Fatalf("Error while opening stream and calling FindMaximum: %v", err)
	}

	waitc := make(chan struct{})

	// send go routine
	go func() {
		numbers := []int32{4, 7, 2, 19, 4, 6, 32}
		for _, number := range numbers {
			fmt.Printf("Sending number: %v\n", number)
			stream.Send(&calculatorpb.FindMaximumRequest{
				Number: number,
			})
			time.Sleep(1000 * time.Millisecond)
		}
		stream.CloseSend()
	}()
	// receive go routine
	go func() {
		for {
			res, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Fatalf("Problem while reading server stream: %v", err)
				break
			}
			maximum := res.GetMaximum()
			fmt.Printf("Received a new maximum of...: %v\n", maximum)
		}
		close(waitc)
	}()
	<-waitc
}

func doErrorUnary(c calculatorpb.CalculatorServiceClient) {
	fmt.Println("Starting to do a SquareRoot Unary RPC...")

	// correct call
	doErrorCall(c, 10)

	// error call
	doErrorCall(c, -2)
}

func doErrorCall(c calculatorpb.CalculatorServiceClient, n int32) {
	res, err := c.SquareRoot(context.Background(), &calculatorpb.SquareRootRequest{Number: n})

	if err != nil {
		respErr, ok := status.FromError(err)
		if ok {
			// actual error from gRPC (user error)
			fmt.Printf("Error message from server: %v\n", respErr.Message())
			fmt.Println(respErr.Code())
			if respErr.Code() == codes.InvalidArgument {
				fmt.Println("We probably sent a negative number!")
				return
			}
		} else {
			log.Fatalf("Big Error calling SquareRoot: %v", err)
			return
		}
	}
	fmt.Printf("Result of square root of %v: %v\n", n, res.GetNumberRoot())
}

func GetConnection(ctx context.Context, target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	var dialOpts []grpc.DialOption
	var customOption *Option

	for _, opt := range opts {
		if co, ok := opt.(Option); ok {
			customOption = &co
			continue
		}

		// detected using WithBlock (restricted options)
		if fmt.Sprintf("%v", opt) == fmt.Sprintf("%v", grpc.WithBlock()) {
			continue
		}

		dialOpts = append(dialOpts, opt)
	}

	if customOption == nil {
		customOption = &defaultOption
	}

	// enforce always using MaxBackoff
	if customOption.MaxBackoff <= 0 {
		customOption.MaxBackoff = defaultOption.MaxBackoff
	}

	if !customOption.Secure {
		dialOpts = append(dialOpts, grpc.WithInsecure())
	}

	// Mandatory customizable: max backoff
	backoffBase := backoff.DefaultConfig
	backoffBase.MaxDelay = customOption.MaxBackoff
	dialOpts = append(dialOpts, grpc.WithConnectParams(grpc.ConnectParams{
		Backoff: backoffBase,
	}))

	// Mandatory: add latest version of client balancer
	// this version will avoid error `there is no address available` that happen on
	// the old version (WithBalancer)
	resolver.SetDefaultScheme("dns")
	dialOpts = append(dialOpts, grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"`+roundrobin.Name+`"}`))

	// Optional: add interceptor waitForReady
	// read detail usage on interceptor func
	// Highly recommend to activate this options
	// just disable this option for specific reason
	if !customOption.FailFast {
		dialOpts = append(dialOpts, grpc.WithChainUnaryInterceptor(waitForReadyInterceptor))
	}

	return grpc.DialContext(ctx, target, dialOpts...)
}
